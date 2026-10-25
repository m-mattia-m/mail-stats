package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/oauthtest"
)

func newManager(t *testing.T, fake *oauthtest.Server) *Manager {
	return &Manager{
		Authority:  fake.URL,
		HTTPClient: fake.Client,
		TokenDir:   filepath.Join(t.TempDir(), "tokens"),
		RunCommand: func(cmd string) (string, error) {
			if cmd == "fail" {
				return "", errors.New("exit 1")
			}
			return cmd + "\n", nil
		},
	}
}

func hotmail() *config.Context {
	return &config.Context{
		Name: "hotmail", Host: "outlook.office365.com", Username: "me@hotmail.com",
		Auth: config.AuthOAuth2, OAuth2: config.OAuth2{ClientID: oauthtest.ClientID},
	}
}

func writeToken(t *testing.T, m *Manager, name string, tok *oauth2.Token) {
	t.Helper()
	if err := m.save(name, tok); err != nil {
		t.Fatal(err)
	}
}

func TestLogin_DeviceFlow(t *testing.T) {
	fake := oauthtest.Start(t)
	m := newManager(t, fake)
	var out bytes.Buffer

	if err := m.Login(context.Background(), hotmail(), &out); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !strings.Contains(out.String(), "open "+oauthtest.VerificationURI+" in a browser and enter the code "+oauthtest.UserCode) {
		t.Errorf("instructions = %q", out.String())
	}

	reqs := fake.Requests()
	if len(reqs) != 2 || reqs[0].Tenant != DefaultTenant || reqs[0].Path != "oauth2/v2.0/devicecode" {
		t.Fatalf("requests = %+v", reqs)
	}
	if got := reqs[0].Form.Get("scope"); got != "https://outlook.office.com/IMAP.AccessAsUser.All offline_access" {
		t.Errorf("scope = %q", got)
	}

	info, err := os.Stat(m.Path("hotmail"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("token file = %v, %v", info, err)
	}
	tok, err := m.Load("hotmail")
	if err != nil || tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("stored token = %+v, %v", tok, err)
	}
}

func TestLogin_Errors(t *testing.T) {
	fake := oauthtest.Start(t)
	m := newManager(t, fake)
	ctx := context.Background()
	var out bytes.Buffer

	c := hotmail()
	c.OAuth2.TokenCommand = "print-token"
	if err := m.Login(ctx, c, &out); err == nil || !strings.Contains(err.Error(), "nothing to log in") {
		t.Errorf("tokenCommand: %v", err)
	}

	c = hotmail()
	c.OAuth2.ClientID = ""
	if err := m.Login(ctx, c, &out); err == nil || !strings.Contains(err.Error(), "no oauth2.clientId") {
		t.Errorf("no client id: %v", err)
	}

	fake.Set(func(s *oauthtest.Server) { s.FailDeviceCode = true })
	if err := m.Login(ctx, hotmail(), &out); err == nil || !strings.Contains(err.Error(), "start device login") {
		t.Errorf("device code failure: %v", err)
	}

	fake.Set(func(s *oauthtest.Server) { s.FailDeviceCode, s.OmitRefreshToken = false, true })
	if err := m.Login(ctx, hotmail(), &out); err == nil || !strings.Contains(err.Error(), "no refresh token") {
		t.Errorf("no refresh token: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.Login(canceled, hotmail(), &out); err == nil {
		t.Error("canceled: want error")
	}
}

func TestAccessToken_ValidTokenNeedsNoRequest(t *testing.T) {
	fake := oauthtest.Start(t)
	m := newManager(t, fake)
	writeToken(t, m, "hotmail", &oauth2.Token{AccessToken: "still-valid", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)})

	tok, err := m.AccessToken(context.Background(), hotmail())
	if err != nil || tok != "still-valid" {
		t.Fatalf("AccessToken = %q, %v", tok, err)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestAccessToken_RefreshesAndStoresRotatedToken(t *testing.T) {
	fake := oauthtest.Start(t)
	m := newManager(t, fake)
	if err := m.Login(context.Background(), hotmail(), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	expired, _ := m.Load("hotmail")
	expired.Expiry = time.Now().Add(-time.Minute)
	writeToken(t, m, "hotmail", expired)

	c := hotmail()
	c.OAuth2.Tenant = "organizations"
	c.OAuth2.Scopes = []string{"custom.scope", "offline_access"}
	tok, err := m.AccessToken(context.Background(), c)
	if err != nil || tok != "access-2" {
		t.Fatalf("AccessToken = %q, %v", tok, err)
	}
	last := fake.Requests()[len(fake.Requests())-1]
	if last.Tenant != "organizations" || last.Form.Get("grant_type") != "refresh_token" || last.Form.Get("refresh_token") != "refresh-1" {
		t.Errorf("refresh request = %+v", last)
	}
	stored, _ := m.Load("hotmail")
	if stored.RefreshToken != "refresh-2" || stored.AccessToken != "access-2" {
		t.Errorf("stored = %+v", stored)
	}
}

func TestAccessToken_Errors(t *testing.T) {
	fake := oauthtest.Start(t)
	m := newManager(t, fake)
	ctx := context.Background()

	if _, err := m.AccessToken(ctx, hotmail()); err == nil || !strings.Contains(err.Error(), "run `mail-stats auth login hotmail`") {
		t.Errorf("not logged in: %v", err)
	}

	// The fake never issued this refresh token.
	writeToken(t, m, "hotmail", &oauth2.Token{AccessToken: "old", RefreshToken: "revoked", Expiry: time.Now().Add(-time.Hour)})
	if _, err := m.AccessToken(ctx, hotmail()); err == nil || !strings.Contains(err.Error(), "refreshing the access token failed") {
		t.Errorf("refresh failure: %v", err)
	}

	if err := os.WriteFile(m.Path("hotmail"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AccessToken(ctx, hotmail()); err == nil || !strings.Contains(err.Error(), "parse token") {
		t.Errorf("corrupt file: %v", err)
	}

	if err := os.Remove(m.Path("hotmail")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(m.Path("hotmail"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Load("hotmail"); err == nil || !strings.Contains(err.Error(), "read token") {
		t.Errorf("unreadable: %v", err)
	}
}

func TestAccessToken_TokenCommand(t *testing.T) {
	m := newManager(t, oauthtest.Start(t))
	ctx := context.Background()
	c := hotmail()

	c.OAuth2.TokenCommand = "the-token"
	if tok, err := m.AccessToken(ctx, c); err != nil || tok != "the-token" {
		t.Errorf("token = %q, %v", tok, err)
	}
	c.OAuth2.TokenCommand = "fail"
	if _, err := m.AccessToken(ctx, c); err == nil || !strings.Contains(err.Error(), "tokenCommand failed") {
		t.Errorf("failing command: %v", err)
	}
	m.RunCommand = func(string) (string, error) { return " \n", nil }
	if _, err := m.AccessToken(ctx, c); err == nil || !strings.Contains(err.Error(), "printed no token") {
		t.Errorf("empty output: %v", err)
	}
}

func TestLogoutAndSaveErrors(t *testing.T) {
	m := newManager(t, oauthtest.Start(t))
	if err := m.Logout("hotmail"); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("logout without token: %v", err)
	}
	writeToken(t, m, "hotmail", &oauth2.Token{AccessToken: "a"})
	if err := m.Logout("hotmail"); err != nil {
		t.Errorf("logout: %v", err)
	}
	if _, err := m.Load("hotmail"); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("load after logout: %v", err)
	}

	// Context names are escaped, so they can't leave the token directory.
	if got := filepath.Base(m.Path("../evil")); got != "..%2Fevil.json" {
		t.Errorf("escaped path = %q", got)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m.TokenDir = filepath.Join(file, "tokens")
	if err := m.save("x", &oauth2.Token{}); err == nil || !strings.Contains(err.Error(), "create token directory") {
		t.Errorf("save below a file: %v", err)
	}
	m.TokenDir = t.TempDir()
	if err := os.Mkdir(m.Path("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := m.save("x", &oauth2.Token{}); err == nil || !strings.Contains(err.Error(), "write token") {
		t.Errorf("save onto a directory: %v", err)
	}
}

func TestDefaults(t *testing.T) {
	m := &Manager{}
	cfg := m.oauthConfig(hotmail())
	if cfg.Endpoint.TokenURL != DefaultAuthority+"/consumers/oauth2/v2.0/token" {
		t.Errorf("token URL = %s", cfg.Endpoint.TokenURL)
	}
	if ctx := context.Background(); m.withClient(ctx) != ctx {
		t.Error("withClient without HTTPClient changed the context")
	}
	data, _ := json.Marshal(cfg.Scopes)
	if string(data) != `["https://outlook.office.com/IMAP.AccessAsUser.All","offline_access"]` {
		t.Errorf("scopes = %s", data)
	}
}
