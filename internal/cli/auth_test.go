package cli

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/imaptest"
	"github.com/m-mattia-m/mail-stats/internal/oauthtest"
)

// withOAuthServer starts an IMAP server that accepts the first access token
// the fake identity platform issues, and adds the OAuth2 context "hotmail".
func (h *harness) withOAuthServer() {
	s := imaptest.Start(h.t, "none", imaptest.WithXOAUTH2("access-1"))
	jan := func(d int) time.Time { return time.Date(2024, 1, d, 10, 0, 0, 0, time.UTC) }
	s.AddFolder(h.t, "INBOX", jan(1), jan(2))
	s.AddFolder(h.t, "Sent", jan(3))
	h.mustRun("config", "set-context", "hotmail",
		"--host", s.Host, "--port", strconv.Itoa(s.Port), "--username", imaptest.Username, "--security", "none",
		"--auth", "oauth2", "--oauth2-client-id", oauthtest.ClientID)
}

func TestAuth_LoginThenStats(t *testing.T) {
	h := newHarness(t)
	h.withOAuthServer()

	h.mustFail("not logged in: run `mail-stats auth login hotmail`", "get", "stats")
	if out := h.mustRun("auth", "status"); !hasRow(out, "hotmail", "username", "auth", "login", "not", "logged", "in") {
		t.Fatalf("status before login:\n%s", out)
	}

	out := h.mustRun("auth", "login")
	for _, want := range []string{
		"open " + oauthtest.VerificationURI + " in a browser and enter the code " + oauthtest.UserCode,
		`Logged in to context "hotmail". Token stored in `,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("login output missing %q:\n%s", want, out)
		}
	}
	if out := h.mustRun("auth", "status"); !hasRow(out, "hotmail", "username", "auth", "login", "logged", "in") {
		t.Fatalf("status after login:\n%s", out)
	}

	if out := h.mustRun("get", "stats"); !hasRow(out, "sent", "1") || !hasRow(out, "received", "2") {
		t.Fatalf("stats:\n%s", out)
	}
	if h.dialed[0].AccessToken != "access-1" || h.dialed[0].Password != "" {
		t.Errorf("dialed = %+v", h.dialed[0])
	}
	if out := h.mustRun("config", "get-contexts"); !hasRow(out, "*", "hotmail", "127.0.0.1") || !strings.Contains(out, "oauth2") {
		t.Errorf("get-contexts:\n%s", out)
	}

	if out := h.mustRun("auth", "logout", "hotmail"); out != "Logged out of context \"hotmail\".\n" {
		t.Errorf("logout = %q", out)
	}
	h.mustFail(`context "hotmail" is not logged in`, "auth", "logout", "--context", "hotmail")
}

func TestAuth_TokenCommandAndErrors(t *testing.T) {
	h := newHarness(t)
	h.mustFail("no context selected", "auth", "login")
	h.mustFail("parse config", "auth", "status", "--config", writeBadConfig(t))

	h.mustRun("config", "set-context", "pw", "--host", "h", "--username", "u")
	h.mustFail(`context "pw" uses password login`, "auth", "login", "pw")
	h.mustFail("needs oauth2.clientId", "config", "set-context", "pw", "--auth", "oauth2")

	h.mustRun("config", "set-context", "cmd", "--host", "h", "--username", "u",
		"--auth", "oauth2", "--oauth2-token-command", "print-token", "--oauth2-tenant", "organizations",
		"--oauth2-scope", "a", "--oauth2-scope", "offline_access", "--oauth2-client-id", "")
	if out := h.mustRun("auth", "status"); !hasRow(out, "cmd", "u", "tokenCommand", "-") {
		t.Fatalf("status:\n%s", out)
	}
	h.mustFail("nothing to log in", "auth", "login", "cmd")

	// The harness's RunCommand refuses everything.
	h.mustFail("oauth2.tokenCommand failed", "get", "stats", "--context", "cmd")

	cfg, err := config.Load(h.configPath)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := cfg.Get("cmd")
	if c.OAuth2.Tenant != "organizations" || len(c.OAuth2.Scopes) != 2 || c.OAuth2.TokenCommand != "print-token" {
		t.Fatalf("oauth2 config = %+v", c.OAuth2)
	}

	// An unreadable token file shows up in the status.
	h.mustRun("config", "set-context", "broken", "--host", "h", "--username", "u", "--auth", "oauth2", "--oauth2-client-id", "x")
	if err := os.MkdirAll(oauthManager(h.deps(nil, nil), h.configPath).Path("broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if out := h.mustRun("auth", "status"); !hasRow(out, "broken", "u", "auth", "login", "error:") {
		t.Fatalf("status with broken token:\n%s", out)
	}

	// Login fails at the fake identity platform: unknown client id.
	h.mustFail("start device login", "auth", "login", "broken")
}

func writeBadConfig(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/bad.yaml"
	if err := os.WriteFile(path, []byte("contexts: [::"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
