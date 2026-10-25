// Package imaptest runs an in-memory IMAP server for tests.
package imaptest

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"github.com/emersion/go-sasl"
)

// Credentials of the single user of the in-memory backend.
const (
	Username = "username"
	Password = "password"
)

// Server is a running in-memory IMAP server.
type Server struct {
	Host string
	Port int
	user backend.User
}

// Option configures Start.
type Option func(*server.Server, backend.Backend)

// WithXOAUTH2 lets clients authenticate with AUTHENTICATE XOAUTH2 using
// token as access token for the user Username.
func WithXOAUTH2(token string) Option {
	return func(srv *server.Server, be backend.Backend) {
		srv.EnableAuth("XOAUTH2", func(conn server.Conn) sasl.Server {
			return &xoauth2Server{token: token, login: func(username string) error {
				if username != Username {
					return errors.New("unknown user")
				}
				user, err := be.Login(conn.Info(), Username, Password)
				if err != nil {
					return err
				}
				ctx := conn.Context()
				ctx.State = imap.AuthenticatedState
				ctx.User = user
				return nil
			}}
		})
	}
}

// xoauth2Server is the server side of SASL XOAUTH2.
type xoauth2Server struct {
	token string
	login func(username string) error
}

func (s *xoauth2Server) Next(response []byte) ([]byte, bool, error) {
	if response == nil {
		return []byte{}, false, nil // ask for the initial response
	}
	var user, auth string
	for _, kv := range strings.Split(string(response), "\x01") {
		if v, ok := strings.CutPrefix(kv, "user="); ok {
			user = v
		}
		if v, ok := strings.CutPrefix(kv, "auth="); ok {
			auth = v
		}
	}
	if auth != "Bearer "+s.token {
		return nil, true, errors.New("invalid access token")
	}
	return nil, true, s.login(user)
}

// Start serves an empty mailbox (only an empty INBOX) on 127.0.0.1. security
// is "tls" (implicit TLS), "starttls" or "none"; TLS uses a self-signed
// certificate, so clients must skip verification. The server is stopped when
// the test ends.
func Start(t testing.TB, security string, opts ...Option) *Server {
	t.Helper()

	be := memory.New()
	user, err := be.Login(nil, Username, Password)
	if err != nil {
		t.Fatalf("login to memory backend: %v", err)
	}

	srv := server.New(be)
	srv.ErrorLog = nopLogger{}
	for _, opt := range opts {
		opt(srv, be)
	}
	var l net.Listener
	switch security {
	case "tls":
		l, err = tls.Listen("tcp", "127.0.0.1:0", tlsConfig(t))
	case "starttls":
		srv.TLSConfig = tlsConfig(t)
		l, err = net.Listen("tcp", "127.0.0.1:0")
	default:
		srv.AllowInsecureAuth = true
		l, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	s := &Server{Host: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port, user: user}
	s.clearInbox(t)
	return s
}

// AddFolder creates folder (unless it exists, like INBOX) and appends one
// message per date, which becomes the message's INTERNALDATE.
func (s *Server) AddFolder(t testing.TB, name string, dates ...time.Time) {
	t.Helper()
	mbox, err := s.user.GetMailbox(name)
	if err != nil {
		if err := s.user.CreateMailbox(name); err != nil {
			t.Fatalf("create mailbox %q: %v", name, err)
		}
		if mbox, err = s.user.GetMailbox(name); err != nil {
			t.Fatalf("get mailbox %q: %v", name, err)
		}
	}
	for _, d := range dates {
		body := bytes.NewBufferString("Subject: test\r\n\r\nhello")
		if err := mbox.CreateMessage(nil, d, body); err != nil {
			t.Fatalf("append to %q: %v", name, err)
		}
	}
}

func (s *Server) clearInbox(t testing.TB) {
	t.Helper()
	inbox, err := s.user.GetMailbox("INBOX")
	if err != nil {
		t.Fatalf("get INBOX: %v", err)
	}
	seq, _ := imap.ParseSeqSet("1:*")
	if err := inbox.UpdateMessagesFlags(false, seq, imap.AddFlags, []string{imap.DeletedFlag}); err != nil {
		t.Fatalf("flag INBOX: %v", err)
	}
	if err := inbox.Expunge(); err != nil {
		t.Fatalf("expunge INBOX: %v", err)
	}
}

func tlsConfig(t testing.TB) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "imaptest"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...interface{}) {}
func (nopLogger) Println(...interface{})        {}
