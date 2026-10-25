package mailbox

import "testing"

func TestClassify(t *testing.T) {
	cls := Classifier{SentFolders: []string{"Archive/Outgoing"}, ExcludeFolders: []string{"Newsletters", "Sent"}}
	tests := []struct {
		name  string
		delim string
		attrs []string
		want  Role
	}{
		{"INBOX", "/", nil, RoleReceived},
		{"Junk", "/", []string{"\\Junk"}, RoleReceived},
		{"Trash", "/", []string{"\\Trash", "\\HasNoChildren"}, RoleReceived},
		{"Projects/Customer A", "/", nil, RoleReceived},
		{"Archive", "/", []string{"\\Archive"}, RoleReceived},
		{"Sent Items", "/", nil, RoleSent},
		{"INBOX.Sent Messages", ".", nil, RoleSent},
		{"INBOX.Gesendet", ".", nil, RoleSent},
		{"[Gmail]/Sent Mail", "/", []string{"\\Sent"}, RoleSent},
		{"Outbox Copy", "/", []string{"\\sent"}, RoleSent},
		{"archive/outgoing", "/", nil, RoleSent},
		{"Drafts", "/", nil, RoleExcluded},
		{"Entwürfe", "/", nil, RoleExcluded},
		{"Meine Entwürfe", "/", []string{"\\Drafts"}, RoleExcluded},
		{"[Gmail]", "/", []string{"\\Noselect"}, RoleExcluded},
		{"[Gmail]/All Mail", "/", []string{"\\All"}, RoleExcluded},
		{"[Gmail]/Starred", "/", []string{"\\Flagged"}, RoleExcluded},
		{"[Gmail]/Important", "/", []string{"\\Important"}, RoleExcluded},
		{"newsletters", "/", nil, RoleExcluded},
		// The user's exclusion beats the name-based sent detection.
		{"Sent", "/", nil, RoleExcluded},
		{"sent", "", nil, RoleSent},
	}
	for _, tt := range tests {
		c := cls
		if tt.delim == "" {
			c = Classifier{}
		}
		if got := c.Classify(tt.name, tt.delim, tt.attrs); got != tt.want {
			t.Errorf("Classify(%q, %v) = %s, want %s", tt.name, tt.attrs, got, tt.want)
		}
	}
}
