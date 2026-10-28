package cli

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/m-mattia-m/mail-stats/internal/imaptest"
)

// withBusyServer adds the context "busy": 2024-01-05 has 1 sent and 3
// received mails, 2024-01-06 has 2 sent.
func (h *harness) withBusyServer() {
	s := imaptest.Start(h.t, "none")
	at := func(d, hour int) time.Time { return time.Date(2024, 1, d, hour, 0, 0, 0, time.UTC) }
	s.AddFolder(h.t, "INBOX", at(5, 9), at(5, 10), at(5, 11), at(20, 9))
	s.AddFolder(h.t, "Sent", at(5, 12), at(6, 9), at(6, 10))
	h.env["IMAP_PW"] = imaptest.Password
	h.mustRun("config", "set-context", "busy",
		"--host", s.Host, "--port", strconv.Itoa(s.Port), "--username", imaptest.Username,
		"--security", "none", "--password-env", "IMAP_PW")
}

func TestGetStats_BusiestDayTable(t *testing.T) {
	h := newHarness(t)
	h.withBusyServer()

	out := h.mustRun("get", "stats", "--context", "busy")
	for _, want := range [][]string{
		{"BUSIEST", "DAY:", "2024-01-05", "(4", "mails:", "1", "sent,", "3", "received)"},
		{"sent", "3"},
		{"received", "4"},
	} {
		if !hasRow(out, want...) {
			t.Errorf("output has no row %v:\n%s", want, out)
		}
	}
	// Last two columns of the direction rows: BUSIEST DAY.
	if !hasRowEnding(out, "sent", "2024-01-06", "(2)") || !hasRowEnding(out, "received", "2024-01-05", "(3)") {
		t.Errorf("per-direction busiest days missing:\n%s", out)
	}
}

func TestGetStats_BusiestDayJSONAndMulti(t *testing.T) {
	h := newHarness(t)
	h.withServer()
	h.withBusyServer()

	var single statsOutput
	if err := json.Unmarshal([]byte(h.mustRun("get", "stats", "--context", "busy", "-o", "json")), &single); err != nil {
		t.Fatal(err)
	}
	b := single.Stats.BusiestDay
	if b == nil || b.Date != "2024-01-05" || b.Total != 4 || b.Sent != 1 || b.Received != 3 {
		t.Fatalf("busiestDay = %+v", b)
	}
	if d := single.Stats.Sent.BusiestDay; d == nil || d.Date != "2024-01-06" || d.Count != 2 {
		t.Fatalf("sent.busiestDay = %+v", d)
	}

	// Combined: the 5th of the "busy" account (4 mails) beats every day of
	// "test" (at most 1 mail per day); "test" adds nothing on the 5th.
	out := h.mustRun("get", "stats", "-A")
	for _, want := range [][]string{
		{"CONTEXT", "ACCOUNT", "PERIOD", "BUSIEST", "DAY"},
		{"(combined)", "2", "accounts", "2024-01-01", "–", "2024-03-01", "(60", "days)", "2024-01-05", "(4", "mails:", "1", "sent,", "3", "received)"},
	} {
		if !hasRow(out, want...) {
			t.Errorf("output has no row %v:\n%s", want, out)
		}
	}
	if !hasRowEnding(out, "(combined)", "2024-01-06", "(2)") {
		t.Errorf("combined sent busiest day missing:\n%s", out)
	}
}

// hasRowEnding reports whether out has a line starting with first and ending
// with the fields last.
func hasRowEnding(out, first string, last ...string) bool {
	for _, line := range splitLines(out) {
		if len(line) <= len(last) || line[0] != first {
			continue
		}
		tail := line[len(line)-len(last):]
		match := true
		for i := range last {
			if tail[i] != last[i] {
				match = false
			}
		}
		if match {
			return true
		}
	}
	return false
}

func splitLines(out string) [][]string {
	var lines [][]string
	for _, l := range strings.Split(out, "\n") {
		lines = append(lines, strings.Fields(l))
	}
	return lines
}
