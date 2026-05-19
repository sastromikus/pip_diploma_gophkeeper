package transport

import (
	"context"
	"net"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type authTestServer struct {
	gophkeeperv1.UnimplementedAuthServiceServer
	t *testing.T
}

func (authTestServer) Register(_ context.Context, request *gophkeeperv1.RegisterRequest) (*gophkeeperv1.RegisterResponse, error) {
	return gophkeeperv1.RegisterResponse_builder{Session: gophkeeperv1.Session_builder{Token: ptr("registered"), ExpiresAt: timestamppb.New(time.Unix(100, 0))}.Build()}.Build(), nil
}
func (authTestServer) Login(_ context.Context, _ *gophkeeperv1.LoginRequest) (*gophkeeperv1.LoginResponse, error) {
	version := uint32(1)
	return gophkeeperv1.LoginResponse_builder{Session: gophkeeperv1.Session_builder{Token: ptr("logged-in"), ExpiresAt: timestamppb.New(time.Unix(200, 0))}.Build(), EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: &version}.Build(), nil
}
func (authTestServer) Logout(ctx context.Context, _ *gophkeeperv1.LogoutRequest) (*gophkeeperv1.LogoutResponse, error) {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 || values[0] != "Bearer token" {
		return nil, status.Error(codes.Unauthenticated, "missing authorization")
	}
	return gophkeeperv1.LogoutResponse_builder{}.Build(), nil
}
func ptr[T any](value T) *T { return &value }
func TestClientAuthMethods(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	gophkeeperv1.RegisterAuthServiceServer(server, authTestServer{t: t})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	connection, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	client := &Client{connection: connection, auth: gophkeeperv1.NewAuthServiceClient(connection)}
	envelope := clientcrypto.KeyEnvelope{EncryptedDataKey: []byte{1}, Salt: []byte{2}, Nonce: []byte{3}, KeyDerivationVersion: 1}
	registeredToken, _, _, err := client.Register(context.Background(), "alice", "password", envelope)
	if err != nil || registeredToken != "registered" {
		t.Fatalf("Register() token = %q, error = %v", registeredToken, err)
	}
	loginToken, _, loginEnvelope, err := client.Login(context.Background(), "alice", "password")
	if err != nil || loginToken != "logged-in" || loginEnvelope.KeyDerivationVersion != 1 {
		t.Fatalf("Login() token = %q envelope = %+v error = %v", loginToken, loginEnvelope, err)
	}
	if err := client.Logout(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
}
