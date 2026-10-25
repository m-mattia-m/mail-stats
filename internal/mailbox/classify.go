package mailbox

import (
	"strings"

	"github.com/emersion/go-imap"
)

// Role says how a folder's mails are counted.
type Role string

const (
	// RoleSent folders count towards sent mails.
	RoleSent Role = "sent"
	// RoleReceived folders (inbox, junk, trash, custom, ...) count towards
	// received mails.
	RoleReceived Role = "received"
	// RoleExcluded folders are not counted at all: drafts, folders that cannot
	// be selected, and virtual folders that only mirror mails stored elsewhere
	// (Gmail's "All Mail", "Starred", "Important").
	RoleExcluded Role = "excluded"
)

// knownSentNames are the leaf names of sent folders on servers that do not
// advertise the RFC 6154 \Sent attribute. Compared case-insensitively.
var knownSentNames = map[string]bool{
	"sent":               true,
	"sent items":         true,
	"sent messages":      true,
	"sent mail":          true,
	"gesendet":           true,
	"gesendete elemente": true,
	"gesendete objekte":  true,
	"envoyés":            true,
	"éléments envoyés":   true,
	"messages envoyés":   true,
	"inviata":            true,
	"posta inviata":      true,
	"enviados":           true,
	"elementos enviados": true,
}

// knownDraftNames are the leaf names of draft folders on servers that do not
// advertise the \Drafts attribute.
var knownDraftNames = map[string]bool{
	"drafts":     true,
	"draft":      true,
	"entwürfe":   true,
	"brouillons": true,
	"bozze":      true,
	"borradores": true,
}

var excludedAttrs = []string{
	imap.NoSelectAttr,
	"\\NonExistent",
	imap.DraftsAttr,
	imap.AllAttr,
	imap.FlaggedAttr,
	imap.ImportantAttr,
}

// Classifier decides the Role of a folder. SentFolders and ExcludeFolders are
// full folder names from the user's config and take precedence over the
// detection by attribute and name.
type Classifier struct {
	SentFolders    []string
	ExcludeFolders []string
}

// Classify returns the role of the folder name (hierarchy delimiter delim)
// with the LIST attributes attrs.
func (c Classifier) Classify(name, delim string, attrs []string) Role {
	if containsFold(c.ExcludeFolders, name) {
		return RoleExcluded
	}
	if containsFold(c.SentFolders, name) {
		return RoleSent
	}
	for _, a := range excludedAttrs {
		if containsFold(attrs, a) {
			return RoleExcluded
		}
	}
	if containsFold(attrs, imap.SentAttr) {
		return RoleSent
	}

	leaf := strings.ToLower(leafName(name, delim))
	switch {
	case knownSentNames[leaf]:
		return RoleSent
	case knownDraftNames[leaf]:
		return RoleExcluded
	default:
		return RoleReceived
	}
}

func leafName(name, delim string) string {
	if delim == "" {
		return name
	}
	parts := strings.Split(name, delim)
	return parts[len(parts)-1]
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
