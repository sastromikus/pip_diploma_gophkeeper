package transport

import (
	"context"
	"net"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type vaultTestServer struct {
	gophkeeperv1.UnimplementedVaultServiceServer
	record *gophkeeperv1.Record
}

func (server vaultTestServer) CreateRecord(ctx context.Context, _ *gophkeeperv1.CreateRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	if values := metadata.ValueFromIncomingContext(ctx, "authorization"); len(values) != 1 || values[0] != "Bearer token" {
		return nil, status.Error(codes.Unauthenticated, "missing authorization")
	}
	return gophkeeperv1.RecordResponse_builder{Record: server.record}.Build(), nil
}
func (server vaultTestServer) GetRecord(context.Context, *gophkeeperv1.GetRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	return gophkeeperv1.RecordResponse_builder{Record: server.record}.Build(), nil
}
func (server vaultTestServer) ListRecords(context.Context, *gophkeeperv1.ListRecordsRequest) (*gophkeeperv1.ListRecordsResponse, error) {
	next, more := server.record.GetId(), true
	return gophkeeperv1.ListRecordsResponse_builder{Records: []*gophkeeperv1.Record{server.record}, NextPageToken: &next, HasMore: &more}.Build(), nil
}
func (server vaultTestServer) UpdateRecord(context.Context, *gophkeeperv1.UpdateRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	return gophkeeperv1.RecordResponse_builder{Record: server.record}.Build(), nil
}
func (server vaultTestServer) DeleteRecord(context.Context, *gophkeeperv1.DeleteRecordRequest) (*gophkeeperv1.DeleteRecordResponse, error) {
	return gophkeeperv1.DeleteRecordResponse_builder{Record: server.record}.Build(), nil
}

func TestClientVaultMethods(t *testing.T) {
	id := "123e4567-e89b-42d3-a456-426614174000"
	typ := gophkeeperv1.RecordType_RECORD_TYPE_TEXT
	encVersion := uint32(1)
	version, revision := int64(2), int64(3)
	now := timestamppb.New(time.Now().UTC())
	record := gophkeeperv1.Record_builder{
		Id: &id, Type: &typ, EncryptionVersion: &encVersion,
		EncryptedPayload: []byte{1}, EncryptedMetadata: []byte{2}, PayloadNonce: []byte{3}, MetadataNonce: []byte{4},
		Version: &version, Revision: &revision, CreatedAt: now, UpdatedAt: now,
	}.Build()

	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	gophkeeperv1.RegisterVaultServiceServer(server, vaultTestServer{record: record})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{connection: connection, vault: gophkeeperv1.NewVaultServiceClient(connection)}
	parsedID, _ := model.ParseID(id)
	data := clientcrypto.EncryptedRecordData{Type: model.RecordTypeText, EncryptionVersion: 1, EncryptedPayload: []byte{1}, EncryptedMetadata: []byte{2}, PayloadNonce: []byte{3}, MetadataNonce: []byte{4}}

	if _, err := client.CreateRecord(context.Background(), "token", parsedID, data); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRecord(context.Background(), "token", parsedID); err != nil {
		t.Fatal(err)
	}
	page, err := client.ListRecords(context.Background(), "token", "", 10)
	if err != nil || len(page.Records) != 1 || !page.HasMore {
		t.Fatalf("ListRecords() = %#v, %v", page, err)
	}
	if _, err := client.UpdateRecord(context.Background(), "token", parsedID, 1, data); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteRecord(context.Background(), "token", parsedID, 1); err != nil {
		t.Fatal(err)
	}
}

func TestClientVaultRejectsMissingAuthorizationAndInvalidMetadata(t *testing.T) {
	client := &Client{}
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	if _, err := client.GetRecord(context.Background(), "", id); err == nil {
		t.Fatal("GetRecord() accepted an empty token")
	}
	if _, err := client.UpdateRecord(context.Background(), "token", id, 0, clientcrypto.EncryptedRecordData{}); err == nil {
		t.Fatal("UpdateRecord() accepted an invalid version")
	}
}
