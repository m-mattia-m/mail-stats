// Package oauth gets OAuth2 access tokens for IMAP: the Microsoft identity
// platform's device-code login, a refresh-token store on disk, and the
// external tokenCommand alternative.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"

	"github.com/m-mattia-m/mail-stats/internal/config"
)

// DefaultAuthority is the base URL of the Microsoft identity platform.
const DefaultAuthority = "https://login.microsoftonline.com"

// DefaultTenant is the Microsoft tenant of personal accounts (Outlook.com,
// Hotmail, Live, MSN).
const DefaultTenant = "consumers"

// DefaultScopes grant IMAP access, and offline_access gets a refresh token so
// the login survives the one-hour access token lifetime.
var DefaultScopes = []string{"https://outlook.office.com/IMAP.AccessAsUser.All", "offline_access"}

// Manager logs contexts in and hands out their access tokens.
type Manager struct {
	// Authority is the identity platform base URL, DefaultAuthority if empty.
	Authority string
	// HTTPClient is used for all token requests, http.DefaultClient if nil.
	HTTPClient *http.Client
	// TokenDir holds one <context>.json token file per logged-in context.
	TokenDir string
	// RunCommand runs a context's tokenCommand.
	RunCommand config.RunCommand
}

// ErrNotLoggedIn is returned for a context without a stored token.
var ErrNotLoggedIn = errors.New("not logged in")

func (m *Manager) oauthConfig(c *config.Context) *oauth2.Config {
	authority := m.Authority
	if authority == "" {
		authority = DefaultAuthority
	}
	tenant := c.OAuth2.Tenant
	if tenant == "" {
		tenant = DefaultTenant
	}
	scopes := c.OAuth2.Scopes
	if len(scopes) == 0 {
		scopes = DefaultScopes
	}
	base := strings.TrimRight(authority, "/") + "/" + url.PathEscape(tenant) + "/oauth2/v2.0"
	return &oauth2.Config{
		ClientID: c.OAuth2.ClientID,
		Scopes:   scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:       base + "/authorize",
			TokenURL:      base + "/token",
			DeviceAuthURL: base + "/devicecode",
			// A public client has no secret; this also skips x/oauth2's
			// probing of both styles.
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func (m *Manager) withClient(ctx context.Context) context.Context {
	if m.HTTPClient == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, m.HTTPClient)
}

// Login runs the device-code flow for c: it prints a URL and a code to out,
// waits until the user has signed in in a browser, and stores the token.
func (m *Manager) Login(ctx context.Context, c *config.Context, out io.Writer) error {
	if c.OAuth2.TokenCommand != "" {
		return fmt.Errorf("context %q gets its tokens from oauth2.tokenCommand, there is nothing to log in", c.Name)
	}
	if c.OAuth2.ClientID == "" {
		return fmt.Errorf("context %q has no oauth2.clientId", c.Name)
	}
	cfg := m.oauthConfig(c)
	ctx = m.withClient(ctx)

	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("start device login: %w", err)
	}
	fmt.Fprintf(out, "To sign in as %s, open %s in a browser and enter the code %s\n", c.Username, da.VerificationURI, da.UserCode)
	fmt.Fprintln(out, "Waiting for the sign-in to complete...")

	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return fmt.Errorf("device login: %w", err)
	}
	if tok.RefreshToken == "" {
		return errors.New("the identity provider returned no refresh token: keep offline_access in oauth2.scopes")
	}
	return m.save(c.Name, tok)
}

// AccessToken returns a valid access token for c, from its tokenCommand or
// from the stored token, refreshing (and re-storing) it when it has expired.
func (m *Manager) AccessToken(ctx context.Context, c *config.Context) (string, error) {
	if cmd := c.OAuth2.TokenCommand; cmd != "" {
		out, err := m.RunCommand(cmd)
		if err != nil {
			return "", fmt.Errorf("context %q: oauth2.tokenCommand failed: %w", c.Name, err)
		}
		token := strings.TrimSpace(out)
		if token == "" {
			return "", fmt.Errorf("context %q: oauth2.tokenCommand printed no token", c.Name)
		}
		return token, nil
	}

	stored, err := m.Load(c.Name)
	if errors.Is(err, ErrNotLoggedIn) {
		return "", fmt.Errorf("context %q is not logged in: run `mail-stats auth login %s`", c.Name, c.Name)
	}
	if err != nil {
		return "", err
	}
	tok, err := m.oauthConfig(c).TokenSource(m.withClient(ctx), stored).Token()
	if err != nil {
		return "", fmt.Errorf("context %q: refreshing the access token failed, run `mail-stats auth login %s` again: %w", c.Name, c.Name, err)
	}
	// Microsoft rotates the refresh token on every refresh.
	if tok.AccessToken != stored.AccessToken || tok.RefreshToken != stored.RefreshToken {
		if err := m.save(c.Name, tok); err != nil {
			return "", err
		}
	}
	return tok.AccessToken, nil
}

// Logout deletes the stored token of the context called name.
func (m *Manager) Logout(name string) error {
	err := os.Remove(m.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotLoggedIn
	}
	return err
}

// Load returns the stored token of the context called name.
func (m *Manager) Load(name string) (*oauth2.Token, error) {
	data, err := os.ReadFile(m.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token %s: %w", m.Path(name), err)
	}
	return &tok, nil
}

// Path returns the token file of the context called name.
func (m *Manager) Path(name string) string {
	return filepath.Join(m.TokenDir, url.PathEscape(name)+".json")
}

func (m *Manager) save(name string, tok *oauth2.Token) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.TokenDir, 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	if err := os.WriteFile(m.Path(name), data, 0o600); err != nil {
		return fmt.Errorf("write token: %w", err)
	}
	return nil
}
