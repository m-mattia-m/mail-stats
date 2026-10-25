// Package mailbox reads folders and mail timestamps from an IMAP server.
package mailbox

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Connection security modes.
const (
	SecurityTLS      = "tls"
	SecurityStartTLS = "starttls"
	SecurityNone     = "none"
)

// Account holds everything needed to log in to an IMAP server.
type Account struct {
	Host     string
	Port     int
	Username string
	Password string
	// AccessToken switches the login from username/password to OAuth2
	// (AUTHENTICATE XOAUTH2), as Microsoft requires.
	AccessToken        string
	Security           string
	InsecureSkipVerify bool
	Timeout            time.Duration
}

// FolderInfo is a folder as returned by the server's LIST response.
type FolderInfo struct {
	Name       string
	Delimiter  string
	Attributes []string
}

// Source is what Collect needs from a mailbox connection.
type Source interface {
	Folders() ([]FolderInfo, error)
	Dates(folder string) ([]time.Time, error)
}

// Client is a logged-in IMAP connection.
type Client struct {
	c *client.Client
}

// Dial connects to the account's server and logs in.
func Dial(acc Account) (*Client, error) {
	addr := net.JoinHostPort(acc.Host, strconv.Itoa(acc.Port))
	dialer := &net.Dialer{Timeout: acc.Timeout}
	tlsConfig := &tls.Config{ServerName: acc.Host, InsecureSkipVerify: acc.InsecureSkipVerify} //nolint:gosec // opt-in via config

	var (
		c   *client.Client
		err error
	)
	switch acc.Security {
	case SecurityTLS, "":
		c, err = client.DialWithDialerTLS(dialer, addr, tlsConfig)
	case SecurityStartTLS, SecurityNone:
		c, err = client.DialWithDialer(dialer, addr)
	default:
		return nil, fmt.Errorf("unknown security mode %q (want tls, starttls or none)", acc.Security)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	c.Timeout = acc.Timeout

	if acc.Security == SecurityStartTLS {
		if err := c.StartTLS(tlsConfig); err != nil {
			_ = c.Terminate()
			return nil, fmt.Errorf("starttls: %w", err)
		}
	}
	if acc.AccessToken != "" {
		if err := c.Authenticate(xoauth2{username: acc.Username, token: acc.AccessToken}); err != nil {
			_ = c.Terminate()
			return nil, fmt.Errorf("login as %s with OAuth2: %w", acc.Username, err)
		}
	} else if err := c.Login(acc.Username, acc.Password); err != nil {
		_ = c.Terminate()
		return nil, fmt.Errorf("login as %s: %w", acc.Username, err)
	}
	return &Client{c: c}, nil
}

// xoauth2 is the SASL XOAUTH2 mechanism used by Microsoft and Google:
// https://learn.microsoft.com/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth
type xoauth2 struct {
	username, token string
}

func (a xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

// Next answers the JSON error the server sends as challenge when the token is
// rejected; the empty response makes it finish with the tagged NO.
func (a xoauth2) Next([]byte) ([]byte, error) {
	return []byte{}, nil
}

// Folders lists all folders of the account.
func (cl *Client) Folders() ([]FolderInfo, error) {
	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() { done <- cl.c.List("", "*", ch) }()

	var folders []FolderInfo
	for m := range ch {
		folders = append(folders, FolderInfo{Name: m.Name, Delimiter: m.Delimiter, Attributes: m.Attributes})
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return folders, nil
}

// Dates returns the INTERNALDATE (the time the server received or stored the
// mail) of every message in folder.
func (cl *Client) Dates(folder string) ([]time.Time, error) {
	status, err := cl.c.Select(folder, true)
	if err != nil {
		return nil, fmt.Errorf("select %q: %w", folder, err)
	}
	if status.Messages == 0 {
		return nil, nil
	}

	seq := new(imap.SeqSet)
	seq.AddRange(1, status.Messages)
	ch := make(chan *imap.Message, 256)
	done := make(chan error, 1)
	go func() { done <- cl.c.Fetch(seq, []imap.FetchItem{imap.FetchInternalDate}, ch) }()

	dates := make([]time.Time, 0, status.Messages)
	for m := range ch {
		dates = append(dates, m.InternalDate)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("fetch %q: %w", folder, err)
	}
	return dates, nil
}

// Close logs out and closes the connection.
func (cl *Client) Close() error {
	if err := cl.c.Logout(); err != nil && !errors.Is(err, client.ErrAlreadyLoggedOut) {
		return err
	}
	return nil
}

// Folder is a classified folder with its message count.
type Folder struct {
	Name     string `json:"name" yaml:"name"`
	Role     Role   `json:"role" yaml:"role"`
	Messages int    `json:"messages" yaml:"messages"`
}

// Collection is everything read from a mailbox.
type Collection struct {
	Folders  []Folder
	Sent     []time.Time
	Received []time.Time
}

// Collect classifies every folder of src and gathers the timestamps of the
// sent and received ones. Excluded folders are listed but not read.
func Collect(src Source, cls Classifier) (*Collection, error) {
	infos, err := src.Folders()
	if err != nil {
		return nil, err
	}

	col := &Collection{}
	for _, info := range infos {
		f := Folder{Name: info.Name, Role: cls.Classify(info.Name, info.Delimiter, info.Attributes)}
		if f.Role != RoleExcluded {
			dates, err := src.Dates(info.Name)
			if err != nil {
				return nil, err
			}
			f.Messages = len(dates)
			if f.Role == RoleSent {
				col.Sent = append(col.Sent, dates...)
			} else {
				col.Received = append(col.Received, dates...)
			}
		}
		col.Folders = append(col.Folders, f)
	}
	return col, nil
}
