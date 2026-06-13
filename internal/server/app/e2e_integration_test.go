package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	clientstorage "github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/config"
	serverpostgres "github.com/sastromikus/gophkeeper/internal/server/storage/postgres"
)

func TestEndToEndTwoClientSynchronization(t *testing.T) {
	dsn := os.Getenv("GOPHKEEPER_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("GOPHKEEPER_TEST_DATABASE_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	address := reserveTCPAddress(t)
	serverCtx, stopServer := context.WithCancel(ctx)
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- Run(serverCtx, e2eServerConfig(address, dsn), slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	waitForTCPServer(t, address, serverResult)

	login := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	password := "correct horse battery staple"
	t.Cleanup(func() {
		stopServer()
		select {
		case err := <-serverResult:
			if err != nil {
				t.Errorf("stop e2e server: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("e2e server did not stop")
		}
		cleanupE2EUser(t, dsn, login)
	})

	first := newE2EClient(t, ctx, address, "first")
	defer first.close(t)
	if err := first.auth.Register(ctx, login, password); err != nil {
		t.Fatalf("register first client: %v", err)
	}

	created, err := first.vault.Create(ctx, password, model.RecordTypeCredentials, clientmodel.Credentials{
		Name: "example.com", Login: "alice", Password: "initial-secret",
	}, clientmodel.Metadata{Text: "created by first client"})
	if err != nil {
		t.Fatalf("create local record: %v", err)
	}
	if report, err := first.sync.Sync(ctx); err != nil {
		t.Fatalf("sync first client after create: %v", err)
	} else if report.Uploaded != 1 {
		t.Fatalf("first create sync uploaded = %d, want 1", report.Uploaded)
	}

	second := newE2EClient(t, ctx, address, "second")
	defer second.close(t)
	if err := second.auth.Login(ctx, login, password); err != nil {
		t.Fatalf("login second client: %v", err)
	}
	if report, err := second.sync.Sync(ctx); err != nil {
		t.Fatalf("initial second-client sync: %v", err)
	} else if report.Downloaded != 1 {
		t.Fatalf("second initial sync downloaded = %d, want 1", report.Downloaded)
	}

	assertCredentialPassword(t, ctx, second.vault, password, created.ID, "initial-secret")

	_, err = second.vault.Update(ctx, password, created.ID, model.RecordTypeCredentials, clientmodel.Credentials{
		Name: "example.com", Login: "alice", Password: "updated-secret",
	}, clientmodel.Metadata{Text: "updated by second client"})
	if err != nil {
		t.Fatalf("update second-client record: %v", err)
	}
	if report, err := second.sync.Sync(ctx); err != nil {
		t.Fatalf("sync second client after update: %v", err)
	} else if report.Uploaded != 1 {
		t.Fatalf("second update sync uploaded = %d, want 1", report.Uploaded)
	}
	if _, err := first.sync.Sync(ctx); err != nil {
		t.Fatalf("download second-client update on first client: %v", err)
	}
	assertCredentialPassword(t, ctx, first.vault, password, created.ID, "updated-secret")

	if err := second.vault.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete record on second client: %v", err)
	}
	if report, err := second.sync.Sync(ctx); err != nil {
		t.Fatalf("sync second client after deletion: %v", err)
	} else if report.Uploaded != 1 {
		t.Fatalf("second deletion sync uploaded = %d, want 1", report.Uploaded)
	}
	if _, err := first.sync.Sync(ctx); err != nil {
		t.Fatalf("download deletion on first client: %v", err)
	}
	if _, err := first.vault.Get(ctx, password, created.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("get deleted record error = %v, want ErrNotFound", err)
	}

	if err := second.auth.Logout(ctx); err != nil {
		t.Fatalf("logout second client: %v", err)
	}
	if err := first.auth.Logout(ctx); err != nil {
		t.Fatalf("logout first client: %v", err)
	}
}

type e2eClient struct {
	transport *clienttransport.Client
	database  *clientstorage.LocalDatabase
	auth      *clientapp.AuthService
	vault     *clientapp.LocalVaultService
	sync      *clientapp.SyncService
}

func newE2EClient(t *testing.T, ctx context.Context, address, name string) *e2eClient {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	sessionStore, err := clientstorage.NewFileSessionStore(filepath.Join(root, "session.json"))
	if err != nil {
		t.Fatalf("create %s session store: %v", name, err)
	}
	database, err := clientstorage.OpenLocalDatabase(ctx, filepath.Join(root, "vault.db"))
	if err != nil {
		t.Fatalf("open %s local database: %v", name, err)
	}
	transport, err := clienttransport.Dial(ctx, clienttransport.Config{Address: address, Insecure: true})
	if err != nil {
		_ = database.Close()
		t.Fatalf("dial %s client: %v", name, err)
	}
	cryptoService := clientcrypto.NewService()
	authService, err := clientapp.NewAuthService(transport, sessionStore, cryptoService)
	if err != nil {
		_ = transport.Close()
		_ = database.Close()
		t.Fatalf("create %s auth service: %v", name, err)
	}
	vaultService, err := clientapp.NewLocalVaultService(sessionStore, database, cryptoService)
	if err != nil {
		_ = transport.Close()
		_ = database.Close()
		t.Fatalf("create %s vault service: %v", name, err)
	}
	syncService, err := clientapp.NewSyncService(transport, sessionStore, database)
	if err != nil {
		_ = transport.Close()
		_ = database.Close()
		t.Fatalf("create %s sync service: %v", name, err)
	}
	return &e2eClient{transport: transport, database: database, auth: authService, vault: vaultService, sync: syncService}
}

func (client *e2eClient) close(t *testing.T) {
	t.Helper()
	if err := client.transport.Close(); err != nil {
		t.Errorf("close e2e transport: %v", err)
	}
	if err := client.database.Close(); err != nil {
		t.Errorf("close e2e local database: %v", err)
	}
}

func assertCredentialPassword(t *testing.T, ctx context.Context, vault *clientapp.LocalVaultService, password string, id model.ID, want string) {
	t.Helper()
	view, err := vault.Get(ctx, password, id)
	if err != nil {
		t.Fatalf("get synchronized credentials: %v", err)
	}
	credentials, ok := view.Payload.(clientmodel.Credentials)
	if !ok {
		t.Fatalf("payload type = %T, want client model Credentials", view.Payload)
	}
	if credentials.Password != want {
		t.Fatalf("credentials password = %q, want %q", credentials.Password, want)
	}
}

func e2eServerConfig(address, dsn string) config.Config {
	return config.Config{
		Address:                  address,
		DatabaseDSN:              dsn,
		Insecure:                 true,
		SessionTTL:               time.Hour,
		ShutdownTimeout:          5 * time.Second,
		MaxEncryptedPayloadSize:  15 << 20,
		MaxEncryptedMetadataSize: 65 << 10,
		AuthRateLimit:            20,
		AuthRateWindow:           time.Minute,
	}
}

func cleanupE2EUser(t *testing.T, dsn, login string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := serverpostgres.Open(ctx, dsn)
	if err != nil {
		t.Errorf("open database for e2e cleanup: %v", err)
		return
	}
	defer database.Close()
	if _, err := database.Pool().Exec(ctx, `DELETE FROM users WHERE login = $1`, login); err != nil {
		t.Errorf("delete e2e user: %v", err)
	}
}
