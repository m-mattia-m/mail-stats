package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	t.Setenv(EnvConfig, "/custom/config.yaml")
	if p, err := DefaultPath(); err != nil || p != "/custom/config.yaml" {
		t.Fatalf("DefaultPath with env = %q, %v", p, err)
	}

	t.Setenv(EnvConfig, "")
	t.Setenv("HOME", "/home/me")
	if p, err := DefaultPath(); err != nil || p != filepath.Join("/home/me", ".mail-stats", "config.yaml") {
		t.Fatalf("DefaultPath = %q, %v", p, err)
	}

	t.Setenv("HOME", "")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("DefaultPath without home: want error")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	cfg, err := Load(path)
	if err != nil || len(cfg.Contexts) != 0 {
		t.Fatalf("Load missing file = %+v, %v", cfg, err)
	}

	cfg.Upsert(Context{Name: "a", Host: "imap.a", Username: "u", SentFolders: []string{"Out"}})
	cfg.CurrentContext = "a"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentContext != "a" || len(loaded.Contexts) != 1 || loaded.Contexts[0].SentFolders[0] != "Out" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("contexts: [::"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Errorf("invalid yaml: err = %v", err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Errorf("directory: err = %v", err)
	}
}

func TestSaveErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	if err := cfg.Save(filepath.Join(file, "config.yaml")); err == nil || !strings.Contains(err.Error(), "create config directory") {
		t.Errorf("parent is a file: err = %v", err)
	}
	if err := cfg.Save(dir); err == nil || !strings.Contains(err.Error(), "write config") {
		t.Errorf("path is a directory: err = %v", err)
	}
}

func TestUpsertDeleteResolve(t *testing.T) {
	cfg := &Config{}
	cfg.Upsert(Context{Name: "a", Host: "one"})
	cfg.Upsert(Context{Name: "b", Host: "two"})
	cfg.Upsert(Context{Name: "a", Host: "three"})
	if len(cfg.Contexts) != 2 || cfg.Contexts[0].Host != "three" {
		t.Fatalf("contexts = %+v", cfg.Contexts)
	}

	if _, err := cfg.Resolve(""); err == nil || !strings.Contains(err.Error(), "no context selected") {
		t.Errorf("no current: err = %v", err)
	}
	cfg.CurrentContext = "a"
	if c, err := cfg.Resolve(""); err != nil || c.Host != "three" {
		t.Errorf("current = %+v, %v", c, err)
	}
	if c, err := cfg.Resolve("b"); err != nil || c.Host != "two" {
		t.Errorf("named = %+v, %v", c, err)
	}
	if _, err := cfg.Resolve("x"); err == nil {
		t.Error("unknown: want error")
	}

	if err := cfg.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "" || len(cfg.Contexts) != 1 {
		t.Fatalf("after delete = %+v", cfg)
	}
	if err := cfg.Delete("a"); err == nil {
		t.Error("delete twice: want error")
	}
	if err := cfg.Delete("b"); err != nil || len(cfg.Contexts) != 0 {
		t.Fatalf("delete b = %v, %+v", err, cfg)
	}
}

func TestEffectivePort(t *testing.T) {
	tests := []struct {
		ctx  Context
		want int
	}{
		{Context{}, 993},
		{Context{Security: "tls"}, 993},
		{Context{Security: "starttls"}, 143},
		{Context{Security: "none"}, 143},
		{Context{Security: "tls", Port: 1993}, 1993},
	}
	for _, tt := range tests {
		if got := tt.ctx.EffectivePort(); got != tt.want {
			t.Errorf("%+v: port = %d, want %d", tt.ctx, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := Context{Name: "a", Host: "h", Username: "u"}
	for _, ok := range []Context{
		valid,
		{Name: "a", Host: "h", Username: "u", Auth: AuthPassword},
		{Name: "a", Host: "h", Username: "u", Auth: AuthOAuth2, OAuth2: OAuth2{ClientID: "id"}},
		{Name: "a", Host: "h", Username: "u", Auth: AuthOAuth2, OAuth2: OAuth2{Provider: ProviderMicrosoft, TokenCommand: "cmd"}},
	} {
		if err := ok.Validate(); err != nil {
			t.Fatalf("valid %+v: %v", ok, err)
		}
		if ok.UsesOAuth2() != (ok.Auth == AuthOAuth2) {
			t.Errorf("UsesOAuth2(%+v) wrong", ok)
		}
	}
	tests := []struct {
		mutate func(*Context)
		want   string
	}{
		{func(c *Context) { c.Host, c.Username = "", "" }, "missing: host, username"},
		{func(c *Context) { c.Name = "" }, "missing: name"},
		{func(c *Context) { c.Security = "ssl" }, "unknown security"},
		{func(c *Context) { c.Port = 70000 }, "invalid port"},
		{func(c *Context) { c.Auth = "kerberos" }, "unknown auth"},
		{func(c *Context) { c.Auth = AuthOAuth2 }, "needs oauth2.clientId"},
		{func(c *Context) { c.Auth, c.OAuth2 = AuthOAuth2, OAuth2{Provider: "google", ClientID: "x"} }, "unknown oauth2 provider"},
	}
	for _, tt := range tests {
		c := valid
		tt.mutate(&c)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("err = %v, want %q", err, tt.want)
		}
	}
}

func TestResolvePassword(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	var ran []string
	run := func(cmd string) (string, error) {
		ran = append(ran, cmd)
		if cmd == "fail" {
			return "", errors.New("exit 1")
		}
		return "from-command\n", nil
	}

	ctx := Context{Name: "a", Password: "plain", PasswordEnv: "PW", PasswordCommand: "pass show"}

	env[EnvPassword] = "override"
	if p, _ := ctx.ResolvePassword(getenv, run); p != "override" {
		t.Errorf("override = %q", p)
	}
	delete(env, EnvPassword)

	if p, _ := ctx.ResolvePassword(getenv, run); p != "from-command" || len(ran) != 1 {
		t.Errorf("command = %q (ran %v)", p, ran)
	}
	ctx.PasswordCommand = "fail"
	if _, err := ctx.ResolvePassword(getenv, run); err == nil || !strings.Contains(err.Error(), "passwordCommand failed") {
		t.Errorf("command failure: %v", err)
	}
	ctx.PasswordCommand = ""

	if _, err := ctx.ResolvePassword(getenv, run); err == nil || !strings.Contains(err.Error(), "PW is empty") {
		t.Errorf("empty env: %v", err)
	}
	env["PW"] = "from-env"
	if p, _ := ctx.ResolvePassword(getenv, run); p != "from-env" {
		t.Errorf("env = %q", p)
	}
	ctx.PasswordEnv = ""

	if p, _ := ctx.ResolvePassword(getenv, run); p != "plain" {
		t.Errorf("plain = %q", p)
	}
	ctx.Password = ""
	if _, err := ctx.ResolvePassword(getenv, run); err == nil || !strings.Contains(err.Error(), "has no password") {
		t.Errorf("none: %v", err)
	}
}

func TestShellCommand(t *testing.T) {
	if out, err := ShellCommand("printf secret"); err != nil || out != "secret" {
		t.Fatalf("ShellCommand = %q, %v", out, err)
	}
	if _, err := ShellCommand("exit 3"); err == nil {
		t.Fatal("failing command: want error")
	}
}

func TestRedacted(t *testing.T) {
	cfg := &Config{CurrentContext: "a", Contexts: []Context{{Name: "a", Password: "secret"}, {Name: "b", PasswordEnv: "PW"}}}
	r := cfg.Redacted()
	if r.Contexts[0].Password != "REDACTED" || r.Contexts[1].Password != "" || r.CurrentContext != "a" {
		t.Fatalf("redacted = %+v", r)
	}
	if cfg.Contexts[0].Password != "secret" {
		t.Fatal("Redacted modified the original")
	}
}
