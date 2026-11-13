package queries_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every exception names the invariant its caller must preserve. New queries
// cannot inherit an exception just because they live beside a write query.
var audienceExceptions = map[string]string{
	"DeletePageTags":        "publish bookkeeping after a visible artifact lock",
	"AddPageTags":           "publish bookkeeping after a visible artifact lock",
	"DeletePropertyAliases": "publish bookkeeping for this visible locked page by artifact id",
	"AddPropertyAlias":      "publish bookkeeping after visible path and alias collision checks; conflict scoped to artifact id",

	"PageBlobInUse":              "content-hash dedup under the blob lock; returns only a boolean",
	"GetPageAttachmentByName":    "upload replaces a name on a page already found visible and locked",
	"InsertPageAttachment":       "insert after a visible page lookup",
	"DeletePageAttachment":       "delete after a visible page lookup",
	"SweepUnusedPageAttachments": "system sweep; returns ids only",
	"TakeDeckBlobSweep":          "system blob sweep; returns only whether a key is unused",

	"CreateArtifact":              "insert only; audience is chosen by trusted service code",
	"NextADRNumber":               "number allocation, not a page reader; ADRs are shared",
	"ADRNumberTaken":              "number allocation; no private note data",
	"SetArtifactCurrentVersion":   "publish after visible artifact lock",
	"UpdateArtifactMeta":          "write after visible artifact lookup and lock",
	"CreateArtifactVersion":       "write after visible artifact lock",
	"CountPublishedVersions":      "version numbering after visible artifact lock",
	"InsertSection":               "outline write in publishing transaction",
	"GetSectionByAnchor":          "RecordRead/chat refs check parent version visibility first",
	"ListArtifactReaders":         "REST detail checks artifact visibility first",
	"ReviewArtifactVersion":       "REST review checks parent artifact visibility first",
	"MockupGateCandidates":        "mockup-only; never notes",
	"MockupReviewsSince":          "mockup-only; never notes",
	"ChatTurnCitations":           "reads from the authenticated person's own chat",
	"AddAlias":                    "write after visible artifact lock; aliases belong to artifact ids",
	"DeleteAlias":                 "delete only this locked page's moved alias by artifact id",
	"SetNotePath":                 "rename after visible artifact lock; audience follows agent namespaces",
	"DeleteLinksFromSource":       "index write in source publishing transaction",
	"InsertLink":                  "index write; target ids hidden by reader visibility",
	"UnresolvedTargets":           "system reindex only, no output to callers",
	"ResolveLinks":                "system reindex only; source ownership scopes private targets",
	"LinksFromSource":             "read_page and run context check source visibility first; expose only raw text, never target metadata",
	"DecideVersion":               "write after visible suggestion lock",
	"DeleteArtifact":              "delete after visible artifact lookup",
	"RelabelVersion":              "suggestion publishing after visible artifact lock",
	"NoteLinksTo":                 "bookkeeping inside the agent's own visible memory namespace",
	"LinksFromTicket":             "run context resolves each target through a visible artifact lookup; raw text is ticket-authored",
	"ListArtifactCitations":       "REST handler checks artifact visibility first; chat identities scoped to viewer",
	"ListArtifactUnlinkedReaders": "REST handler checks artifact visibility first",
}
var namedQuery = regexp.MustCompile(`(?m)^-- name: (\w+) :\w+`)
var pageTable = regexp.MustCompile(`(?i)\b(artifacts|knowledge_links|artifact_sections|artifact_aliases|artifact_versions|page_tags|page_aliases|page_attachments)\b`)
var sqlComment = regexp.MustCompile(`(?m)--[^\n]*`)

func guarded(name, sql string) bool {
	sql = sqlComment.ReplaceAllString(sql, "")
	return !pageTable.MatchString(sql) || strings.Contains(sql, "artifact_visible(") || audienceExceptions[name] != ""
}
func TestEveryPageQueryHasAudienceFilterOrReason(t *testing.T) {
	files, err := filepath.Glob("*.sql")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		matches := namedQuery.FindAllStringSubmatchIndex(s, -1)
		for i, m := range matches {
			name := s[m[2]:m[3]]
			end := len(s)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			sql := s[m[1]:end]
			seen[name] = true
			if !guarded(name, sql) {
				t.Errorf("%s: %s reads page data without artifact_visible or a documented exception", file, name)
			}
		}
	}
	for name, reason := range audienceExceptions {
		if !seen[name] || strings.TrimSpace(reason) == "" {
			t.Errorf("stale/empty exception: %s", name)
		}
	}
}
func TestAudienceGuardRejectsNewUnfilteredReader(t *testing.T) {
	for _, sql := range []string{"SELECT * FROM artifacts", "SELECT * FROM artifact_versions", "SELECT target_id FROM knowledge_links", "-- artifact_visible(\nSELECT * FROM artifacts"} {
		if guarded("NewAgentReader", sql) {
			t.Fatalf("guard allowed %s", sql)
		}
	}
	if !guarded("VisibleReader", "SELECT * FROM artifacts WHERE artifact_visible(audience, owner_user_id, $1)") {
		t.Fatal("filtered query rejected")
	}
}
