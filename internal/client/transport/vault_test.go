package transport

import (
	"bytes"
	"context"
	"errors"
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

func (server vaultTestServer) SyncRecords(context.Context, *gophkeeperv1.SyncRecordsRequest) (*gophkeeperv1.SyncRecordsResponse, error) {
	next, more := server.record.GetRevision(), false
	return gophkeeperv1.SyncRecordsResponse_builder{Records: []*gophkeeperv1.Record{server.record}, NextRevision: &next, HasMore: &more}.Build(), nil
}

func TestClientVaultMethods(t *testing.T) {
	id := "123e4567-e89b-42d3-a456-426614174000"
	typ := gophkeeperv1.RecordType_RECORD_TYPE_TEXT
	encVersion := uint32(1)
	version, revision := int64(2), int64(3)
	now := timestamppb.New(time.Now().UTC())
	record := gophkeeperv1.Record_builder{
		Id: &id, Type: &typ, EncryptionVersion: &encVersion,
		EncryptedPayload:  bytes.Repeat([]byte{1}, model.RecordAuthenticationTagSize),
		EncryptedMetadata: bytes.Repeat([]byte{2}, model.RecordAuthenticationTagSize),
		PayloadNonce:      bytes.Repeat([]byte{3}, model.RecordNonceSize),
		MetadataNonce:     bytes.Repeat([]byte{4}, model.RecordNonceSize),
		Version:           &version, Revision: &revision, CreatedAt: now, UpdatedAt: now,
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
	data := clientcrypto.EncryptedRecordData{
		Type:              model.RecordTypeText,
		EncryptionVersion: model.CurrentRecordEncryptionVersion,
		EncryptedPayload:  bytes.Repeat([]byte{1}, model.RecordAuthenticationTagSize),
		EncryptedMetadata: bytes.Repeat([]byte{2}, model.RecordAuthenticationTagSize),
		PayloadNonce:      bytes.Repeat([]byte{3}, model.RecordNonceSize),
		MetadataNonce:     bytes.Repeat([]byte{4}, model.RecordNonceSize),
	}

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
	pageSync, err := client.SyncRecords(context.Background(), "token", 0, 10)
	if err != nil || len(pageSync.Records) != 1 || pageSync.NextRevision != revision {
		t.Fatalf("SyncRecords() = %#v, %v", pageSync, err)
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

func TestRemoteRecordRejectsMalformedServerData(t *testing.T) {
	id := "123e4567-e89b-42d3-a456-426614174000"
	typ := gophkeeperv1.RecordType_RECORD_TYPE_TEXT
	encVersion := uint32(model.CurrentRecordEncryptionVersion)
	version, revision := int64(1), int64(1)
	now := timestamppb.New(time.Now().UTC())
	valid := func() *gophkeeperv1.Record {
		return gophkeeperv1.Record_builder{
			Id: &id, Type: &typ, EncryptionVersion: &encVersion,
			EncryptedPayload:  make([]byte, model.RecordAuthenticationTagSize),
			EncryptedMetadata: make([]byte, model.RecordAuthenticationTagSize),
			PayloadNonce:      make([]byte, model.RecordNonceSize),
			MetadataNonce:     make([]byte, model.RecordNonceSize),
			Version:           &version, Revision: &revision, CreatedAt: now, UpdatedAt: now,
		}.Build()
	}

	tests := []struct {
		name    string
		message func() *gophkeeperv1.Record
	}{
		{name: "nil", message: func() *gophkeeperv1.Record { return nil }},
		{name: "unsupported encryption version", message: func() *gophkeeperv1.Record {
			m := valid()
			m.SetEncryptionVersion(model.CurrentRecordEncryptionVersion + 1)
			return m
		}},
		{name: "short payload", message: func() *gophkeeperv1.Record {
			m := valid()
			m.SetEncryptedPayload(make([]byte, model.RecordAuthenticationTagSize-1))
			return m
		}},
		{name: "bad nonce", message: func() *gophkeeperv1.Record {
			m := valid()
			m.SetPayloadNonce(make([]byte, model.RecordNonceSize-1))
			return m
		}},
		{name: "invalid version metadata", message: func() *gophkeeperv1.Record {
			m := valid()
			m.SetVersion(0)
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := remoteRecord(tt.message()); err == nil {
				t.Fatal("remoteRecord() accepted malformed data")
			}
		})
	}
}

func TestMapRPCError(t *testing.T) {
	tests := []struct {
		code codes.Code
		want error
	}{
		{codes.InvalidArgument, model.ErrInvalidInput},
		{codes.NotFound, model.ErrNotFound},
		{codes.AlreadyExists, model.ErrAlreadyExists},
		{codes.Aborted, model.ErrVersionConflict},
		{codes.Unauthenticated, model.ErrUnauthenticated},
		{codes.PermissionDenied, model.ErrForbidden},
		{codes.ResourceExhausted, model.ErrPayloadTooLarge},
	}
	for _, tt := range tests {
		err := mapRPCError(status.Error(tt.code, "test"))
		if !errors.Is(err, tt.want) {
			t.Fatalf("mapRPCError(%v) = %v, want %v", tt.code, err, tt.want)
		}
	}
}
