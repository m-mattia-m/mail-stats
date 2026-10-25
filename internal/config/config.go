// Package config reads and writes the kubeconfig-style file that holds the
// IMAP accounts ("contexts") and which one is used by default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvConfig overrides the config file location.
const EnvConfig = "MAIL_STATS_CONFIG"

// EnvPassword overrides the password of whichever context is used.
const EnvPassword = "MAIL_STATS_PASSWORD"

// Context is one IMAP account.
type Context struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port,omitempty"`
	Username string `yaml:"username"`
	// Security is tls (default, implicit TLS), starttls or none.
	Security string `yaml:"security,omitempty"`
	// Password is stored in plain text; prefer PasswordEnv or PasswordCommand.
	Password string `yaml:"password,omitempty"`
	// PasswordEnv names an environment variable holding the password.
	PasswordEnv string `yaml:"passwordEnv,omitempty"`
	// PasswordCommand is run with `sh -c`; its trimmed stdout is the password
	// (e.g. `op read op://Private/mail/password`).
	PasswordCommand    string   `yaml:"passwordCommand,omitempty"`
	InsecureSkipVerify bool     `yaml:"insecureSkipVerify,omitempty"`
	SentFolders        []string `yaml:"sentFolders,omitempty"`
	ExcludeFolders     []string `yaml:"excludeFolders,omitempty"`
}

// Config is the whole config file.
type Config struct {
	CurrentContext string    `yaml:"currentContext,omitempty"`
	Contexts       []Context `yaml:"contexts"`
}

// DefaultPath returns $MAIL_STATS_CONFIG or ~/.mail-stats/config.yaml.
func DefaultPath() (string, error) {
	if p := os.Getenv(EnvConfig); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".mail-stats", "config.yaml"), nil
}

// Load reads the config at path. A missing file yields an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes the config to path, readable only by the owner since it may
// contain a password.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// Get returns the context called name.
func (c *Config) Get(name string) (*Context, bool) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			return &c.Contexts[i], true
		}
	}
	return nil, false
}

// Upsert adds ctx, or replaces the context with the same name.
func (c *Config) Upsert(ctx Context) {
	if existing, ok := c.Get(ctx.Name); ok {
		*existing = ctx
		return
	}
	c.Contexts = append(c.Contexts, ctx)
}

// Delete removes the context called name and clears CurrentContext if it
// pointed to it.
func (c *Config) Delete(name string) error {
	for i := range c.Contexts {
		if c.Contexts[i].Name == name {
			c.Contexts = append(c.Contexts[:i], c.Contexts[i+1:]...)
			if c.CurrentContext == name {
				c.CurrentContext = ""
			}
			return nil
		}
	}
	return fmt.Errorf("context %q not found", name)
}

// Resolve returns the context called name, or the current context if name is
// empty.
func (c *Config) Resolve(name string) (*Context, error) {
	if name == "" {
		name = c.CurrentContext
	}
	if name == "" {
		return nil, errors.New("no context selected: pass --context or run `mail-stats config use-context <name>`")
	}
	ctx, ok := c.Get(name)
	if !ok {
		return nil, fmt.Errorf("context %q not found", name)
	}
	return ctx, nil
}

// EffectivePort returns Port, or the default port for the security mode.
func (ctx Context) EffectivePort() int {
	if ctx.Port != 0 {
		return ctx.Port
	}
	if ctx.Security == "starttls" || ctx.Security == "none" {
		return 143
	}
	return 993
}

// Validate checks that the context can be used to connect.
func (ctx Context) Validate() error {
	var missing []string
	if ctx.Name == "" {
		missing = append(missing, "name")
	}
	if ctx.Host == "" {
		missing = append(missing, "host")
	}
	if ctx.Username == "" {
		missing = append(missing, "username")
	}
	if len(missing) > 0 {
		return fmt.Errorf("context %q is missing: %s", ctx.Name, strings.Join(missing, ", "))
	}
	switch ctx.Security {
	case "", "tls", "starttls", "none":
	default:
		return fmt.Errorf("context %q: unknown security %q (want tls, starttls or none)", ctx.Name, ctx.Security)
	}
	if ctx.Port < 0 || ctx.Port > 65535 {
		return fmt.Errorf("context %q: invalid port %d", ctx.Name, ctx.Port)
	}
	return nil
}

// Getenv looks up an environment variable.
type Getenv func(string) string

// RunCommand runs a shell command and returns its stdout.
type RunCommand func(command string) (string, error)

// ShellCommand runs command with `sh -c`.
func ShellCommand(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).Output() //nolint:gosec // the user's own config
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ResolvePassword returns the password, in order of precedence, from
// $MAIL_STATS_PASSWORD, PasswordCommand, PasswordEnv, then Password.
func (ctx Context) ResolvePassword(getenv Getenv, run RunCommand) (string, error) {
	if p := getenv(EnvPassword); p != "" {
		return p, nil
	}
	if ctx.PasswordCommand != "" {
		out, err := run(ctx.PasswordCommand)
		if err != nil {
			return "", fmt.Errorf("context %q: passwordCommand failed: %w", ctx.Name, err)
		}
		return strings.TrimRight(out, "\r\n"), nil
	}
	if ctx.PasswordEnv != "" {
		if p := getenv(ctx.PasswordEnv); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("context %q: environment variable %s is empty", ctx.Name, ctx.PasswordEnv)
	}
	if ctx.Password != "" {
		return ctx.Password, nil
	}
	return "", fmt.Errorf("context %q has no password: set passwordEnv, passwordCommand or %s", ctx.Name, EnvPassword)
}

// Redacted returns a copy of the config with plain-text passwords masked.
func (c *Config) Redacted() *Config {
	out := &Config{CurrentContext: c.CurrentContext, Contexts: make([]Context, len(c.Contexts))}
	for i, ctx := range c.Contexts {
		if ctx.Password != "" {
			ctx.Password = "REDACTED"
		}
		out.Contexts[i] = ctx
	}
	return out
}
