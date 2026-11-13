package wiki

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/artifacts"
	"github.com/liamphmurphy/vesper/server/internal/links"
)

const MaxVaultUpload int64 = 2 << 30
const maxVaultEntries = 100000
const maxVaultExpanded uint64 = 8 << 30
const maxVaultEntry uint64 = 512 << 20

type VaultEntry struct {
	Duplicate      bool       `json:"duplicate,omitempty"`
	IndexedAliases []string   `json:"indexed_aliases"`
	OriginalTitle  string     `json:"original_title,omitempty"`
	Source         string     `json:"source"`
	Path           string     `json:"path"`
	Kind           string     `json:"kind"`
	Hash           string     `json:"hash"`
	Size           uint64     `json:"size"`
	Title          string     `json:"title,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	Aliases        []string   `json:"aliases,omitempty"`
	ExistingID     *uuid.UUID `json:"existing_id,omitempty"`
	BeforeID       *uuid.UUID `json:"before_id,omitempty"`
	ExportID       *uuid.UUID `json:"export_id,omitempty"`
	Renamed        bool       `json:"renamed,omitempty"`
	Conflict       bool       `json:"conflict,omitempty"`
	Daily          bool       `json:"daily,omitempty"`
	Error          string     `json:"error,omitempty"`
	Unsupported    bool       `json:"unsupported,omitempty"`
}
type VaultPlan struct {
	Prepared    bool         `json:"prepared,omitempty"`
	Entries     []VaultEntry `json:"entries"`
	Notes       int          `json:"notes"`
	Attachments int          `json:"attachments"`
	Resolved    int          `json:"resolved"`
	Unresolved  []string     `json:"unresolved"`
	Folders     []string     `json:"folders"`
	Tags        []string     `json:"tags"`
}

func safeVaultPath(name string) bool {
	if len(name) > 4096 || strings.ContainsRune(name, 0) || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || strings.Contains(strings.Split(name, "/")[0], ":") {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." || seg == "." {
			return false
		}
	}
	return true
}
func ignoredVaultPath(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") || seg == "__MACOSX" {
			return true
		}
	}
	return false
}
func importPath(name, folder string) string {
	name = strings.TrimSuffix(name, path.Ext(name))
	segments := strings.Split(strings.Trim(folder, "/")+"/"+name, "/")
	clean := []string{}
	for _, seg := range segments {
		seg = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if strings.ContainsRune("[]#|^\\", r) || r < 32 {
				return ' '
			}
			return r
		}, seg)), " ")
		if seg != "" && seg != "." && seg != ".." {
			clean = append(clean, seg)
		}
	}
	if len(clean) == 0 {
		clean = []string{"Untitled"}
	}
	if len(clean) > 0 && (strings.EqualFold(clean[0], "agents") || (len(clean) > 1 && strings.EqualFold(clean[0], "Patterns") && strings.EqualFold(clean[1], "Proposed"))) {
		clean = append([]string{"Imported"}, clean...)
	}

	if len(clean) > 9 {
		clean = append(clean[:8], strings.Join(clean[8:], " - "))
	}
	p := strings.Join(clean, "/")
	if utf8.RuneCountInString(p) > 280 {
		r := []rune(p)
		sum := sha256.Sum256([]byte(p))
		p = string(r[:260]) + fmt.Sprintf(" %x", sum[:8])
	}
	return p
}
func readZipNote(f *zip.File) (string, artifacts.Properties, error) {
	if f.UncompressedSize64 > artifacts.MaxMarkdownBytes {
		return "", nil, fmt.Errorf("note exceeds 2 MB")
	}
	rc, err := f.Open()
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, artifacts.MaxMarkdownBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > artifacts.MaxMarkdownBytes {
		return "", nil, fmt.Errorf("note exceeds 2 MB")
	}
	return splitFrontmatter(string(data))
}

// PlanVault validates the entire directory before opening any content. No
// archive member is ever extracted to a filesystem path or rendered.
func PlanVault(z *zip.Reader, folder string) (VaultPlan, error) {
	out := VaultPlan{Entries: []VaultEntry{}, Unresolved: []string{}, Folders: []string{}, Tags: []string{}}
	if len(z.File) > maxVaultEntries {
		return out, apperr.Invalid("file", "archive has too many entries")
	}
	var expanded uint64
	seen := map[string]bool{}
	prefix := ""
	wrapped := true
	for _, f := range z.File {
		if !safeVaultPath(f.Name) {
			return out, apperr.Invalid("file", "unsafe archive path %q", f.Name)
		}
		if seen[strings.ToLower(f.Name)] {
			return out, apperr.Invalid("file", "duplicate archive path %q", f.Name)
		}
		seen[strings.ToLower(f.Name)] = true
		if f.UncompressedSize64 > maxVaultEntry || f.UncompressedSize64 > maxVaultExpanded-expanded {
			return out, apperr.Invalid("file", "archive expands beyond the allowed size")
		}
		expanded += f.UncompressedSize64
		if f.UncompressedSize64 > 1<<20 && (f.CompressedSize64 == 0 || f.UncompressedSize64/max(1, f.CompressedSize64) > 1000) {
			return out, apperr.Invalid("file", "excessive compression ratio")
		}
		if f.FileInfo().IsDir() || ignoredVaultPath(f.Name) {
			continue
		}
		first, _, ok := strings.Cut(f.Name, "/")
		if !ok {
			wrapped = false
		}
		if prefix == "" {
			prefix = first
		} else if prefix != first {
			wrapped = false
		}
	}
	if _, err := artifacts.ValidNotePath(strings.Trim(folder, "/") + "/placeholder"); folder != "" && err != nil {
		return out, err
	}
	paths := map[string]bool{}
	bodies := map[string]string{}
	tags := map[string]bool{}
	folders := map[string]bool{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() || ignoredVaultPath(f.Name) {
			continue
		}
		source := f.Name
		if wrapped {
			source = strings.TrimPrefix(source, prefix+"/")
		}
		kind := "attachment"
		if strings.EqualFold(path.Ext(source), ".md") && !strings.HasSuffix(strings.ToLower(source), ".excalidraw.md") {
			kind = "note"
		}
		entry := VaultEntry{Source: f.Name, Path: importPath(source, folder), Kind: kind, Size: f.UncompressedSize64, Aliases: []string{}, Tags: []string{}}
		if kind == "note" {
			body, props, err := readZipNote(f)
			if err != nil {
				entry.Error = err.Error()
				out.Entries = append(out.Entries, entry)
				continue
			}
			entry.Title = links.TitleOf(strings.TrimSuffix(source, path.Ext(source)))
			if title, ok := props["title"].(string); ok {
				entry.Title = title
			}
			if len([]rune(entry.Title)) > 200 {
				entry.OriginalTitle = entry.Title
				entry.Title = string([]rune(entry.Title)[:200])
			}
			entry.Aliases, _ = artifacts.StringList(props["aliases"])
			entry.Tags = artifacts.Tags(props, body)
			if id, ok := props["vesper_id"].(string); ok {
				if parsed, err := uuid.Parse(id); err == nil {
					entry.ExportID = &parsed
				}
			}
			if entry.ExportID == nil {
				if id, ok := props["lazyagents_id"].(string); ok {
					if parsed, err := uuid.Parse(id); err == nil {
						entry.ExportID = &parsed
					}
				}
			}
			hash := sha256.Sum256([]byte(body))
			entry.Hash = fmt.Sprintf("%x", hash)
			bodies[f.Name] = body
			base := entry.Path
			for n := 2; paths[strings.ToLower(entry.Path)]; n++ {
				entry.Path = fmt.Sprintf("%s %d", base, n)
			}
			paths[strings.ToLower(entry.Path)] = true
			entry.Renamed = entry.Path != strings.Trim(strings.Trim(folder, "/")+"/"+strings.TrimSuffix(source, path.Ext(source)), "/")
			entry.Daily = isDailyFile(source)
			out.Notes++
		} else {
			rc, err := f.Open()
			if err != nil {
				entry.Error = err.Error()
				out.Entries = append(out.Entries, entry)
				continue
			}
			h := sha256.New()
			n, err := io.Copy(h, io.LimitReader(rc, int64(maxVaultEntry)+1))
			rc.Close()
			if err != nil || uint64(n) != f.UncompressedSize64 {
				entry.Error = "attachment is corrupt or exceeds size limit"
			} else {
				entry.Hash = fmt.Sprintf("%x", h.Sum(nil))
			}
			ext := strings.ToLower(path.Ext(source))
			entry.Unsupported = ext == ".canvas" || ext == ".base" || strings.Contains(strings.ToLower(source), "excalidraw")
			out.Attachments++
		}
		for _, tag := range entry.Tags {
			tags[tag] = true
		}
		if dir := path.Dir(source); dir != "." {
			folders[dir] = true
		}
		out.Entries = append(out.Entries, entry)
	}
	resolver := vaultResolver(out.Entries)
	for name, body := range bodies {
		for _, ref := range links.Parse(body) {
			if ref.Target == "" {
				continue
			}
			if _, ok := resolver(name, ref.Target); ok {
				out.Resolved++
			} else {
				out.Unresolved = append(out.Unresolved, ref.Target)
			}
		}
	}
	for tag := range tags {
		out.Tags = append(out.Tags, tag)
	}
	sort.Strings(out.Tags)
	for f := range folders {
		out.Folders = append(out.Folders, f)
	}
	sort.Strings(out.Folders)
	return out, nil
}

var dailyFile = regexp.MustCompile(`(?i)(?:^|/)(?:daily|daily notes|journal)/\d{4}-\d{2}-\d{2}\.md$`)

func isDailyFile(name string) bool { return dailyFile.MatchString(name) }
func vaultResolver(entries []VaultEntry) func(string, string) (VaultEntry, bool) {
	sorted := append([]VaultEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if len(sorted[i].Source) != len(sorted[j].Source) {
			return len(sorted[i].Source) < len(sorted[j].Source)
		}
		return sorted[i].Source < sorted[j].Source
	})
	return func(from, target string) (VaultEntry, bool) {
		if decoded, err := url.PathUnescape(target); err == nil {
			target = decoded
		}
		target = strings.TrimPrefix(target, "./")
		target = strings.TrimSuffix(target, ".md")
		for _, e := range sorted {
			if e.Error != "" {
				continue
			}
			name := e.Source
			if e.Kind == "note" {
				name = strings.TrimSuffix(name, path.Ext(name))
			}
			if strings.EqualFold(name, target) || strings.EqualFold(e.Path, target) || strings.EqualFold(name, strings.TrimSuffix(path.Join(path.Dir(from), target), ".md")) {
				return e, true
			}
		}
		for _, e := range sorted {
			if e.Error == "" && strings.EqualFold(vaultBase(e), path.Base(target)) {
				return e, true
			}
		}
		for _, e := range sorted {
			for _, alias := range e.Aliases {
				if e.Error == "" && strings.EqualFold(alias, target) {
					return e, true
				}
			}
		}
		return VaultEntry{}, false
	}
}

var markdownFileLink = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(([^)\n]+)\)`)

func rewriteVaultLinks(body, source string, entries []VaultEntry, assets map[string]string) string {
	resolve := vaultResolver(entries)
	refs := links.Parse(body)
	lines := strings.Split(body, "\n")
	// Reverse offsets within each line, so earlier replacements keep their spans.
	for i := len(refs) - 1; i >= 0; i-- {
		ref := refs[i]
		if ref.Target == "" {
			continue
		}
		e, ok := resolve(source, ref.Target)
		if !ok {
			continue
		}
		target := e.Path
		if e.Kind == "attachment" {
			var exists bool
			target, exists = assets[e.Source]
			if !exists {
				continue
			}
		}
		if ref.Anchor != "" {
			target += "#" + ref.Anchor
		}
		if ref.Display != "" {
			target += "|" + ref.Display
		}
		replacement := "[[" + target + "]]"
		if ref.Embed {
			replacement = "!" + replacement
		}
		line := lines[ref.Line]
		lines[ref.Line] = line[:ref.Start] + replacement + line[ref.End:]
	}
	fenced := ""
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			marker := trim[:3]
			if fenced == "" {
				fenced = marker
			} else if fenced == marker {
				fenced = ""
			}
			continue
		}
		if fenced != "" {
			continue
		}
		lines[i] = rewriteMarkdownFileRefs(line, func(raw string) string {
			m := markdownFileLink.FindStringSubmatch(raw)
			target, anchor, _ := strings.Cut(m[3], "#")
			if strings.Contains(target, ":") || strings.HasPrefix(target, "//") {
				return raw
			}
			e, ok := resolve(source, target)
			if !ok {
				return raw
			}
			dest := e.Path
			if e.Kind == "attachment" {
				var exists bool
				dest, exists = assets[e.Source]
				if !exists {
					return raw
				}
			}
			if anchor != "" {
				dest += "#" + anchor
			}
			if m[2] != "" {
				dest += "|" + m[2]
			}
			return m[1] + "[[" + dest + "]]"
		})
	}
	return strings.Join(lines, "\n")
}

func vaultBase(e VaultEntry) string {
	name := path.Base(e.Source)
	if e.Kind == "note" {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}

var vaultInlineCode = regexp.MustCompile("`+[^`\\n]*`+")

func rewriteMarkdownFileRefs(line string, replace func(string) string) string {
	masked := vaultInlineCode.ReplaceAllStringFunc(line, func(s string) string { return strings.Repeat(" ", len(s)) })
	spans := markdownFileLink.FindAllStringIndex(masked, -1)
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		line = line[:span[0]] + replace(line[span[0]:span[1]]) + line[span[1]:]
	}
	return line
}
