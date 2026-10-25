package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/m-mattia-m/mail-stats/internal/config"
	"github.com/m-mattia-m/mail-stats/internal/imaptest"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
	"github.com/m-mattia-m/mail-stats/internal/oauthtest"
)

var now = time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

type harness struct {
	t          *testing.T
	configPath string
	env        map[string]string
	oauth      *oauthtest.Server
	mu         sync.Mutex // Dial runs concurrently for several contexts
	dialed     []mailbox.Account
	dialErr    error
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, configPath: filepath.Join(t.TempDir(), "config.yaml"), env: map[string]string{}, oauth: oauthtest.Start(t)}
}

func (h *harness) deps(out, errOut *bytes.Buffer) Deps {
	return Deps{
		Out:        out,
		Err:        errOut,
		Getenv:     func(k string) string { return h.env[k] },
		RunCommand: func(string) (string, error) { return "", errors.New("not allowed in tests") },
		Now:        func() time.Time { return now },
		Dial: func(acc mailbox.Account) (Session, error) {
			h.mu.Lock()
			h.dialed = append(h.dialed, acc)
			dialErr := h.dialErr
			h.mu.Unlock()
			if dialErr != nil {
				return nil, dialErr
			}
			return mailbox.Dial(acc)
		},
		Version:        "v1.2.3",
		OAuthAuthority: h.oauth.URL,
		HTTPClient:     h.oauth.Client,
	}
}

// run executes the CLI and returns stdout, stderr and the exit code.
func (h *harness) run(args ...string) (string, string, int) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	code := Run(h.deps(&out, &errOut), append([]string{"--config", h.configPath}, args...))
	return out.String(), errOut.String(), code
}

func (h *harness) mustRun(args ...string) string {
	h.t.Helper()
	out, errOut, code := h.run(args...)
	if code != 0 {
		h.t.Fatalf("mail-stats %v: exit %d: %s", args, code, errOut)
	}
	return out
}

func (h *harness) mustFail(want string, args ...string) {
	h.t.Helper()
	_, errOut, code := h.run(args...)
	if code == 0 || !strings.Contains(errOut, want) {
		h.t.Fatalf("mail-stats %v: exit %d, stderr %q, want error containing %q", args, code, errOut, want)
	}
}

// withServer starts an IMAP server with some history and a context for it.
func (h *harness) withServer() *imaptest.Server {
	s := imaptest.Start(h.t, "none")
	jan := func(d int) time.Time { return time.Date(2024, 1, d, 10, 0, 0, 0, time.UTC) }
	s.AddFolder(h.t, "INBOX", jan(1), jan(15), jan(31))
	s.AddFolder(h.t, "Junk", jan(20))
	s.AddFolder(h.t, "Trash", jan(21))
	s.AddFolder(h.t, "Projects/Customer", jan(22))
	s.AddFolder(h.t, "Sent", jan(2), jan(16))
	s.AddFolder(h.t, "Outgoing", jan(17))
	s.AddFolder(h.t, "Drafts", jan(18))

	h.env["IMAP_PW"] = imaptest.Password
	h.mustRun("config", "set-context", "test",
		"--host", s.Host, "--port", strconv.Itoa(s.Port), "--username", imaptest.Username,
		"--security", "none", "--password-env", "IMAP_PW", "--sent-folder", "Outgoing")
	return s
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	if out := h.mustRun("version"); out != "v1.2.3\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestGetStats_Table(t *testing.T) {
	h := newHarness(t)
	h.withServer()

	out := h.mustRun("get", "stats")
	for _, want := range []string{
		"CONTEXT:   test (username @ 127.0.0.1)",
		"PERIOD:    2024-01-01 – 2024-03-01 (60 days)",
		"DIRECTION   TOTAL   PER DAY   PER WEEK   PER MONTH   FIRST        LAST",
		"sent        3       0.05      0.35       1.52        2024-01-02   2024-01-17",
		"received    6       0.10      0.70       3.04        2024-01-01   2024-01-31",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(h.dialed) != 1 || h.dialed[0].Password != imaptest.Password || h.dialed[0].Timeout != 30*time.Second {
		t.Errorf("dialed = %+v", h.dialed)
	}
}

func TestGetStats_JSONWithWindow(t *testing.T) {
	h := newHarness(t)
	h.withServer()

	out := h.mustRun("get", "stats", "-o", "json", "--since", "2024-01-15", "--until", "2024-01-20", "--context", "test", "--timeout", "3s")
	var got statsOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if got.Context != "test" || got.Stats.Sent.Total != 2 || got.Stats.Received.Total != 2 {
		t.Fatalf("stats = %+v", got)
	}
	if len(got.Folders) != 7 {
		t.Errorf("folders = %+v", got.Folders)
	}
	if h.dialed[0].Timeout != 3*time.Second {
		t.Errorf("timeout = %v", h.dialed[0].Timeout)
	}
}

func TestGetStats_YAML(t *testing.T) {
	h := newHarness(t)
	h.withServer()

	out := h.mustRun("get", "stat", "-o", "yaml")
	var got map[string]any
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid yaml: %v\n%s", err, out)
	}
	if got["context"] != "test" {
		t.Fatalf("yaml = %v", got)
	}
}

func TestGetStats_Errors(t *testing.T) {
	h := newHarness(t)
	h.mustFail("unknown output format", "get", "stats", "-o", "xml")
	h.mustFail("invalid --since", "get", "stats", "--since", "yesterday")
	h.mustFail("invalid --until", "get", "stats", "--until", "2024-13-01")
	h.mustFail("no context selected", "get", "stats")

	h.withServer()
	h.mustFail(`context "nope" not found`, "get", "stats", "--context", "nope")

	delete(h.env, "IMAP_PW")
	h.mustFail("IMAP_PW is empty", "get", "stats")
	h.env["IMAP_PW"] = "wrong"
	h.mustFail("login as username", "get", "stats")

	h.env["IMAP_PW"] = imaptest.Password
	h.dialErr = errors.New("network down")
	h.mustFail("network down", "get", "stats")
}

func TestGetStats_InvalidContextAndConfig(t *testing.T) {
	h := newHarness(t)
	cfg := &config.Config{CurrentContext: "broken", Contexts: []config.Context{{Name: "broken"}}}
	if err := cfg.Save(h.configPath); err != nil {
		t.Fatal(err)
	}
	h.mustFail("missing: host, username", "get", "stats")

	if err := os.WriteFile(h.configPath, []byte("contexts: [::"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustFail("parse config", "get", "stats")
	h.mustFail("parse config", "get", "folders")
	h.mustFail("parse config", "config", "view")
	h.mustFail("parse config", "config", "get-contexts")
	h.mustFail("parse config", "config", "current-context")
	h.mustFail("parse config", "config", "use-context", "a")
	h.mustFail("parse config", "config", "set-context", "a")
	h.mustFail("parse config", "config", "delete-context", "a")
}

func TestGetFolders(t *testing.T) {
	h := newHarness(t)
	h.withServer()

	out := h.mustRun("get", "folders")
	for _, want := range []string{
		"NAME                ROLE       MESSAGES",
		"INBOX               received   3",
		"Projects/Customer   received   1",
		"Sent                sent       2",
		"Outgoing            sent       1",
		"Drafts              excluded   -",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	var folders []mailbox.Folder
	if err := json.Unmarshal([]byte(h.mustRun("get", "folders", "-o", "json")), &folders); err != nil || len(folders) != 7 {
		t.Fatalf("json folders = %+v, %v", folders, err)
	}

	h.mustFail("unknown output format", "get", "folders", "-o", "csv")
	h.dialErr = errors.New("network down")
	h.mustFail("network down", "get", "folders")
}

func TestConfigLifecycle(t *testing.T) {
	h := newHarness(t)

	h.mustFail("current-context is not set", "config", "current-context")
	if out := h.mustRun("config", "set-context", "personal", "--host", "imap.example.com", "--username", "me@example.com", "--password-env", "PW"); out != "Context \"personal\" created.\n" {
		t.Fatalf("set-context = %q", out)
	}
	// The first context becomes the current one.
	if out := h.mustRun("config", "current-context"); out != "personal\n" {
		t.Fatalf("current-context = %q", out)
	}

	h.mustRun("config", "set-context", "work", "--host", "mail.work", "--username", "me", "--security", "starttls",
		"--password-command", "op read x", "--insecure-skip-verify", "--exclude-folder", "Spam", "--port", "1143")
	if out := h.mustRun("config", "set-context", "personal", "--port", "1993"); out != "Context \"personal\" modified.\n" {
		t.Fatalf("modify = %q", out)
	}

	out := h.mustRun("config", "get-contexts")
	for _, want := range []string{
		"CURRENT   NAME       HOST               PORT   USERNAME         SECURITY   AUTH",
		"*         personal   imap.example.com   1993   me@example.com   tls        password",
		"          work       mail.work          1143   me               starttls   password",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("get-contexts missing %q:\n%s", want, out)
		}
	}

	if out := h.mustRun("config", "use-context", "work"); out != "Switched to context \"work\".\n" {
		t.Fatalf("use-context = %q", out)
	}
	h.mustFail(`context "nope" not found`, "config", "use-context", "nope")

	cfg, err := config.Load(h.configPath)
	if err != nil {
		t.Fatal(err)
	}
	work, _ := cfg.Get("work")
	if cfg.CurrentContext != "work" || !work.InsecureSkipVerify || work.ExcludeFolders[0] != "Spam" || work.PasswordCommand != "op read x" {
		t.Fatalf("config = %+v", cfg)
	}

	if out := h.mustRun("config", "delete-context", "work"); out != "Deleted context \"work\".\n" {
		t.Fatalf("delete-context = %q", out)
	}
	h.mustFail(`context "work" not found`, "config", "delete-context", "work")
	h.mustFail("missing: host, username", "config", "set-context", "incomplete")
	h.mustFail("accepts 1 arg", "config", "use-context")
}

func TestConfigView(t *testing.T) {
	h := newHarness(t)
	cfg := &config.Config{CurrentContext: "a", Contexts: []config.Context{{Name: "a", Host: "h", Username: "u", Password: "secret"}}}
	if err := cfg.Save(h.configPath); err != nil {
		t.Fatal(err)
	}

	out := h.mustRun("config", "view")
	if strings.Contains(out, "secret") || !strings.Contains(out, "password: REDACTED") || !strings.Contains(out, "currentContext: a") {
		t.Fatalf("view = %s", out)
	}
	if out := h.mustRun("config", "view", "-o", "json"); !strings.Contains(out, `"CurrentContext": "a"`) {
		t.Fatalf("view json = %s", out)
	}
	h.mustFail("unknown output format", "config", "view", "-o", "toml")
}

func TestConfigSaveFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	h := newHarness(t)
	dir := filepath.Dir(h.configPath)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	h.mustFail("write config", "config", "set-context", "a", "--host", "h", "--username", "u")
}

func TestConfigSaveFailure_UseAndDelete(t *testing.T) {
	h := newHarness(t)
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	h.mustRun("config", "set-context", "a", "--host", "h", "--username", "u")
	if err := os.Chmod(h.configPath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(h.configPath, 0o600) })
	h.mustFail("write config", "config", "use-context", "a")
	h.mustFail("write config", "config", "delete-context", "a")
}

func TestDefaultConfigPath(t *testing.T) {
	t.Setenv(config.EnvConfig, filepath.Join(t.TempDir(), "env.yaml"))
	var out, errOut bytes.Buffer
	h := newHarness(t)
	if code := Run(h.deps(&out, &errOut), []string{"config", "set-context", "a", "--host", "h", "--username", "u"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if _, err := os.Stat(os.Getenv(config.EnvConfig)); err != nil {
		t.Fatalf("config not written to $%s: %v", config.EnvConfig, err)
	}

	t.Setenv(config.EnvConfig, "")
	t.Setenv("HOME", "")
	errOut.Reset()
	if code := Run(h.deps(&out, &errOut), []string{"config", "view"}); code == 0 || !strings.Contains(errOut.String(), "home directory") {
		t.Fatalf("without home: exit %d, %s", code, errOut.String())
	}
}

func TestDefaultDeps(t *testing.T) {
	d := DefaultDeps("v9")
	if d.Out != os.Stdout || d.Err != os.Stderr || d.Version != "v9" || d.Now().IsZero() {
		t.Fatalf("DefaultDeps = %+v", d)
	}
	if _, err := d.Dial(mailbox.Account{Host: "127.0.0.1", Port: 1, Security: "bogus"}); err == nil {
		t.Fatal("Dial with bogus security: want error")
	}
}

func TestExecute(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"mail-stats", "--config", filepath.Join(t.TempDir(), "c.yaml"), "config", "get-contexts"}
	if code := Execute("dev"); code != 0 {
		t.Fatalf("Execute = %d", code)
	}
}
