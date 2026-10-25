package mailbox_test

import (
	"errors"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/m-mattia-m/mail-stats/internal/imaptest"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
)

func account(s *imaptest.Server, security string) mailbox.Account {
	return mailbox.Account{
		Host:               s.Host,
		Port:               s.Port,
		Username:           imaptest.Username,
		Password:           imaptest.Password,
		Security:           security,
		InsecureSkipVerify: true,
		Timeout:            5 * time.Second,
	}
}

func TestDial_SecurityModes(t *testing.T) {
	for _, security := range []string{"tls", "starttls", "none"} {
		t.Run(security, func(t *testing.T) {
			s := imaptest.Start(t, security)
			c, err := mailbox.Dial(account(s, security))
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			if _, err := c.Folders(); err != nil {
				t.Fatalf("Folders: %v", err)
			}
			if err := c.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		})
	}
}

func TestDial_DefaultSecurityIsTLS(t *testing.T) {
	s := imaptest.Start(t, "tls")
	c, err := mailbox.Dial(account(s, ""))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = c.Close()
}

func TestDial_Errors(t *testing.T) {
	s := imaptest.Start(t, "none")

	wrongPassword := account(s, "none")
	wrongPassword.Password = "nope"
	if _, err := mailbox.Dial(wrongPassword); err == nil || !strings.Contains(err.Error(), "login as username") {
		t.Errorf("wrong password: err = %v", err)
	}

	if _, err := mailbox.Dial(account(s, "smoke-signals")); err == nil || !strings.Contains(err.Error(), "unknown security") {
		t.Errorf("unknown security: err = %v", err)
	}

	// The plain-text server does not offer STARTTLS.
	if _, err := mailbox.Dial(account(s, "starttls")); err == nil || !strings.Contains(err.Error(), "starttls") {
		t.Errorf("starttls unsupported: err = %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := account(s, "none")
	closed.Port = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	if _, err := mailbox.Dial(closed); err == nil || !strings.Contains(err.Error(), "connect to") {
		t.Errorf("closed port: err = %v", err)
	}
}

func TestClient_FoldersAndDates(t *testing.T) {
	s := imaptest.Start(t, "none")
	d1 := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	d2 := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	s.AddFolder(t, "INBOX", d1, d2)
	s.AddFolder(t, "Sent", d2)
	s.AddFolder(t, "Empty")

	c, err := mailbox.Dial(account(s, "none"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	folders, err := c.Folders()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range folders {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "Empty,INBOX,Sent" {
		t.Fatalf("folders = %v", names)
	}

	dates, err := c.Dates("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || !dates[0].Equal(d1) || !dates[1].Equal(d2) {
		t.Fatalf("INBOX dates = %v", dates)
	}
	if dates, err := c.Dates("Empty"); err != nil || len(dates) != 0 {
		t.Fatalf("Empty dates = %v, %v", dates, err)
	}
	if _, err := c.Dates("Missing"); err == nil || !strings.Contains(err.Error(), `select "Missing"`) {
		t.Fatalf("missing folder: err = %v", err)
	}
}

func TestClient_ErrorsAfterLogout(t *testing.T) {
	s := imaptest.Start(t, "none")
	c, err := mailbox.Dial(account(s, "none"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := c.Folders(); err == nil {
		t.Error("Folders after logout: want error")
	}
}

type fakeSource struct {
	folders    []mailbox.FolderInfo
	dates      map[string][]time.Time
	foldersErr error
	datesErr   error
	read       []string
}

func (f *fakeSource) Folders() ([]mailbox.FolderInfo, error) { return f.folders, f.foldersErr }

func (f *fakeSource) Dates(name string) ([]time.Time, error) {
	f.read = append(f.read, name)
	return f.dates[name], f.datesErr
}

func TestCollect(t *testing.T) {
	now := time.Now()
	src := &fakeSource{
		folders: []mailbox.FolderInfo{
			{Name: "INBOX", Delimiter: "/"},
			{Name: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}},
			{Name: "Trash", Delimiter: "/", Attributes: []string{"\\Trash"}},
			{Name: "Drafts", Delimiter: "/", Attributes: []string{"\\Drafts"}},
			{Name: "Sent Old", Delimiter: "/"},
		},
		dates: map[string][]time.Time{
			"INBOX":    {now, now, now},
			"Sent":     {now},
			"Trash":    {now},
			"Drafts":   {now},
			"Sent Old": {now, now},
		},
	}

	col, err := mailbox.Collect(src, mailbox.Classifier{SentFolders: []string{"Sent Old"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Sent) != 3 || len(col.Received) != 4 {
		t.Fatalf("sent/received = %d/%d, want 3/4", len(col.Sent), len(col.Received))
	}
	want := []mailbox.Folder{
		{Name: "INBOX", Role: mailbox.RoleReceived, Messages: 3},
		{Name: "Sent", Role: mailbox.RoleSent, Messages: 1},
		{Name: "Trash", Role: mailbox.RoleReceived, Messages: 1},
		{Name: "Drafts", Role: mailbox.RoleExcluded},
		{Name: "Sent Old", Role: mailbox.RoleSent, Messages: 2},
	}
	for i, f := range col.Folders {
		if f != want[i] {
			t.Errorf("folder %d = %+v, want %+v", i, f, want[i])
		}
	}
	for _, name := range src.read {
		if name == "Drafts" {
			t.Error("excluded folder was read")
		}
	}
}

func TestCollect_Errors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := mailbox.Collect(&fakeSource{foldersErr: boom}, mailbox.Classifier{}); !errors.Is(err, boom) {
		t.Errorf("folders error = %v", err)
	}
	src := &fakeSource{folders: []mailbox.FolderInfo{{Name: "INBOX"}}, datesErr: boom}
	if _, err := mailbox.Collect(src, mailbox.Classifier{}); !errors.Is(err, boom) {
		t.Errorf("dates error = %v", err)
	}
}
