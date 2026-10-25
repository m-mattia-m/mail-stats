package cli

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m-mattia-m/mail-stats/internal/imaptest"
	"github.com/m-mattia-m/mail-stats/internal/mailbox"
)

// withSecondServer adds the context "other" for a second server whose history
// starts in February.
func (h *harness) withSecondServer() *imaptest.Server {
	s := imaptest.Start(h.t, "none")
	feb := func(d int) time.Time { return time.Date(2024, 2, d, 10, 0, 0, 0, time.UTC) }
	s.AddFolder(h.t, "INBOX", feb(10), feb(20))
	s.AddFolder(h.t, "Sent", feb(15))
	h.addContext("other", s)
	return s
}

func (h *harness) addContext(name string, s *imaptest.Server) {
	h.t.Helper()
	h.env["IMAP_PW"] = imaptest.Password
	h.mustRun("config", "set-context", name,
		"--host", s.Host, "--port", strconv.Itoa(s.Port), "--username", imaptest.Username,
		"--security", "none", "--password-env", "IMAP_PW")
}

// hasRow reports whether out has a line whose whitespace-separated fields
// start with want.
func hasRow(out string, want ...string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < len(want) {
			continue
		}
		match := true
		for i, w := range want {
			if fields[i] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestGetStats_MultipleContextsTable(t *testing.T) {
	h := newHarness(t)
	h.withServer()
	h.withSecondServer()

	out := h.mustRun("get", "stats", "--contexts", "test,other")

	for _, want := range [][]string{
		{"CONTEXT", "ACCOUNT", "PERIOD"},
		{"test", "username", "@", "127.0.0.1", "2024-01-01", "–", "2024-03-01", "(60", "days)"},
		{"other", "username", "@", "127.0.0.1", "2024-02-10", "–", "2024-03-01", "(20", "days)"},
		{"(combined)", "2", "accounts", "2024-01-01", "–", "2024-03-01", "(60", "days)"},
		{"CONTEXT", "DIRECTION", "TOTAL", "PER", "DAY"},
		{"test", "sent", "3", "0.05", "0.35", "1.52", "2024-01-02", "2024-01-17"},
		{"test", "received", "6", "0.10", "0.70", "3.04"},
		{"other", "sent", "1", "0.05", "0.35", "1.00", "2024-02-15", "2024-02-15"},
		{"other", "received", "2", "0.10", "0.70", "2.00", "2024-02-10", "2024-02-20"},
		{"(combined)", "sent", "4", "0.07", "0.47", "2.03", "2024-01-02", "2024-02-15"},
		{"(combined)", "received", "8", "0.13", "0.93", "4.05", "2024-01-01", "2024-02-20"},
	} {
		if !hasRow(out, want...) {
			t.Errorf("output has no row %v:\n%s", want, out)
		}
	}
	if len(h.dialed) != 2 {
		t.Errorf("dialed %d accounts, want 2", len(h.dialed))
	}
}

func TestGetStats_AllContextsJSON(t *testing.T) {
	h := newHarness(t)
	h.withServer()
	h.withSecondServer()

	out := h.mustRun("get", "stats", "-A", "-o", "json", "--since", "2024-02-01")
	var got multiStatsOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if len(got.Contexts) != 2 || got.Contexts[0].Context != "test" || got.Contexts[1].Context != "other" {
		t.Fatalf("contexts = %+v", got.Contexts)
	}
	if got.Contexts[0].Stats.Sent.Total != 0 || got.Contexts[1].Stats.Received.Total != 2 {
		t.Errorf("per-context stats = %+v", got.Contexts)
	}
	if got.Combined.Sent.Total != 1 || got.Combined.Received.Total != 2 {
		t.Errorf("combined = %+v", got.Combined)
	}
	if len(got.Contexts[0].Folders) != 7 {
		t.Errorf("folders = %+v", got.Contexts[0].Folders)
	}
}

func TestGetStats_SingleContextSelectionUsesMultiShape(t *testing.T) {
	h := newHarness(t)
	h.withServer()

	// --contexts with one name, listed twice: still the multi-context shape,
	// with the duplicate dropped.
	out := h.mustRun("get", "stats", "--contexts", "test", "--contexts", "test")
	if !hasRow(out, "(combined)", "1", "accounts") || !hasRow(out, "test", "sent", "3") {
		t.Fatalf("output:\n%s", out)
	}
	if len(h.dialed) != 1 {
		t.Errorf("dialed %d times, want 1", len(h.dialed))
	}
}

func TestGetStats_DuplicateAccountWarning(t *testing.T) {
	h := newHarness(t)
	s := h.withServer()
	h.addContext("alias", s)
	h.mustRun("config", "set-context", "alias", "--sent-folder", "Outgoing")

	out, errOut, code := h.run("get", "stats", "-A")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, `Warning: contexts "test" and "alias" use the same account`) {
		t.Errorf("stderr = %q", errOut)
	}
	if !hasRow(out, "(combined)", "sent", "6") {
		t.Errorf("combined counts both contexts:\n%s", out)
	}
}

func TestGetStats_MultipleContextsErrors(t *testing.T) {
	h := newHarness(t)
	h.mustFail("no contexts configured", "get", "stats", "-A")

	h.withServer()
	h.mustFail("[all-contexts contexts] were all set", "get", "stats", "-A", "--contexts", "test")
	h.mustFail("--context cannot be combined", "get", "stats", "--context", "test", "--contexts", "test")
	h.mustFail("--context cannot be combined", "get", "folders", "--context", "test", "-A")
	h.mustFail(`context "nope" not found`, "get", "stats", "--contexts", "test,nope")

	h.withSecondServer()
	h.dialErr = errors.New("network down")
	h.mustFail(`context "test": network down`, "get", "stats", "-A")

	h.dialErr = nil
	h.mustRun("config", "set-context", "broken", "--host", "h", "--username", "u")
	h.mustFail(`context "broken" has no password`, "get", "stats", "-A")
	h.mustRun("config", "set-context", "broken", "--security", "none", "--port", "1", "--password-env", "IMAP_PW")
	h.mustFail(`context "broken": connect to h:1`, "get", "stats", "-A", "--timeout", "2s")
}

func TestGetFolders_MultipleContexts(t *testing.T) {
	h := newHarness(t)
	h.withServer()
	h.withSecondServer()

	out := h.mustRun("get", "folders", "-A")
	for _, want := range [][]string{
		{"CONTEXT", "NAME", "ROLE", "MESSAGES"},
		{"test", "INBOX", "received", "3"},
		{"test", "Drafts", "excluded", "-"},
		{"other", "INBOX", "received", "2"},
		{"other", "Sent", "sent", "1"},
	} {
		if !hasRow(out, want...) {
			t.Errorf("output has no row %v:\n%s", want, out)
		}
	}

	var got []struct {
		Context string           `json:"context"`
		Folders []mailbox.Folder `json:"folders"`
	}
	if err := json.Unmarshal([]byte(h.mustRun("get", "folders", "--contexts", "other", "-o", "json")), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Context != "other" || len(got[0].Folders) != 2 {
		t.Fatalf("json = %+v", got)
	}
}
