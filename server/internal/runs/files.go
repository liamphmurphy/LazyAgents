package runs

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The Library's `file` kind holds anything that isn't media: code, a zip, a
// spreadsheet, a PDF. It stores the bytes and describes them; it never runs,
// renders or unpacks them. Downloads are attachments, a zip is only listed
// (its central directory is read, no entry is extracted), and executables and
// disk images are refused so the Library can't be used to pass one along.

const (
	// FilePreviewLines and FilePreviewRows are how much of a text or table
	// file a preview shows.
	FilePreviewLines = 200
	FilePreviewRows  = 20
	// FilePreviewEntries is how many names of a zip are listed.
	FilePreviewEntries = 200
	// maxPreviewBytes bounds what a preview reads of a text or table file.
	maxPreviewBytes = 1 << 20
)

// deniedFileExts are executables, installers and disk images.
var deniedFileExts = map[string]bool{
	".exe": true, ".dll": true, ".msi": true, ".scr": true, ".com": true, ".pif": true, ".bat": true, ".cmd": true, ".vbs": true, ".lnk": true,
	".app": true, ".apk": true, ".ipa": true, ".dmg": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true, ".jar": true,
}

// executableMagic starts the files an OS runs: Windows PE, ELF, Mach-O.
var executableMagic = [][]byte{
	[]byte("MZ"), []byte("\x7fELF"),
	{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe},
}

// textExts are source and data files shown as text.
var textExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".py": true, ".go": true, ".js": true, ".mjs": true, ".ts": true, ".tsx": true, ".jsx": true, ".json": true,
	".yaml": true, ".yml": true, ".toml": true, ".sql": true, ".sh": true, ".bash": true, ".rb": true, ".rs": true, ".java": true, ".kt": true, ".swift": true,
	".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true, ".cs": true, ".css": true, ".html": true, ".htm": true, ".xml": true, ".svg": true, ".log": true,
	".ini": true, ".cfg": true, ".conf": true, ".env": true, ".ps1": true, ".lua": true, ".php": true, ".r": true, ".tex": true,
}

// knownFileTypes are the types a few binary formats are served as.
var knownFileTypes = map[string]string{
	".pdf": "application/pdf", ".zip": "application/zip",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xls": "application/vnd.ms-excel",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".csv": "text/csv", ".tsv": "text/tab-separated-values", ".mid": "audio/midi", ".midi": "audio/midi", ".gz": "application/gzip", ".tar": "application/x-tar", ".7z": "application/x-7z-compressed",
}

// FileType is what the Library records for a file: its content type, and an
// error naming why it's refused. Any file is stored and served as an
// attachment, so the type only describes it; it is never rendered in the page.
func FileType(name string, head []byte) (string, error) {
	ext := strings.ToLower(path.Ext(name))
	if deniedFileExts[ext] {
		return "", fmt.Errorf("%s isn't kept: executables and disk images aren't allowed", name)
	}
	for _, m := range executableMagic {
		if bytes.HasPrefix(head, m) {
			return "", fmt.Errorf("%s isn't kept: it's an executable, whatever its name says", name)
		}
	}
	if ct, ok := knownFileTypes[ext]; ok {
		return ct, nil
	}
	if textExts[ext] || (ext == "" && looksLikeText(head)) {
		return "text/plain; charset=utf-8", nil
	}
	if ct := mime.TypeByExtension(ext); ct != "" && !strings.HasPrefix(ct, "text/html") {
		return ct, nil
	}
	if looksLikeText(head) {
		return "text/plain; charset=utf-8", nil
	}
	if ct := http.DetectContentType(head); !strings.HasPrefix(ct, "text/html") {
		return ct, nil
	}
	return "application/octet-stream", nil
}

// looksLikeText reports whether b is UTF-8 text with no NUL bytes.
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	// A cut in the middle of a rune isn't binary.
	for i := 0; i < utf8.UTFMax && i < len(b); i++ {
		if utf8.Valid(b[:len(b)-i]) {
			return true
		}
	}
	return false
}

// FileEntry is one name in a zip.
type FileEntry struct {
	Name string
	Size int64
}

// FilePreview is what a file's dialog shows: lines of text, rows of a table,
// or a zip's entries. Kind is "none" for a file with nothing to show.
type FilePreview struct {
	Kind      string // text, table, zip or none
	Lines     []string
	Rows      [][]string
	Entries   []FileEntry
	Total     int // lines, rows or entries in the whole file
	Truncated bool
}

func previewKind(name string, head []byte) string {
	ext := strings.ToLower(path.Ext(name))
	switch {
	case ext == ".zip":
		return "zip"
	case ext == ".csv" || ext == ".tsv":
		return "table"
	case ext == ".pdf" || ext == ".xlsx" || ext == ".xls" || ext == ".docx" || ext == ".pptx" || ext == ".gz" || ext == ".tar" || ext == ".7z" || ext == ".mid" || ext == ".midi":
		return "none"
	case textExts[ext] || looksLikeText(head):
		return "text"
	}
	return "none"
}

// BuildFilePreview describes the file in r (size bytes). It reads at most a
// megabyte of text, streams a table to count its rows, and for a zip reads only
// the directory.
func BuildFilePreview(name string, r io.ReaderAt, size int64) (FilePreview, error) {
	head := make([]byte, min(size, 512))
	if _, err := r.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return FilePreview{}, err
	}
	switch previewKind(name, head) {
	case "zip":
		return zipPreview(r, size)
	case "table":
		return tablePreview(name, io.NewSectionReader(r, 0, size))
	case "text":
		return textPreview(io.NewSectionReader(r, 0, size), size)
	}
	return FilePreview{Kind: "none"}, nil
}

func textPreview(r io.Reader, size int64) (FilePreview, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxPreviewBytes))
	if err != nil {
		return FilePreview{}, err
	}
	if !utf8.Valid(data) && !looksLikeText(data) {
		return FilePreview{Kind: "none"}, nil
	}
	all := strings.Split(strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")
	out := FilePreview{Kind: "text", Total: len(all), Truncated: int64(len(data)) < size || len(all) > FilePreviewLines}
	if int64(len(data)) == maxPreviewBytes && size > maxPreviewBytes {
		// Only the first megabyte is read; count the rest of the lines without keeping them.
		// The cut may fall mid-line, so count newlines, not the pieces of the split.
		rest, last, err := countNewlines(r)
		if err != nil {
			return FilePreview{}, err
		}
		out.Total = bytes.Count(data, []byte{'\n'}) + rest
		if last != '\n' {
			out.Total++ // the final line has no newline after it
		}
	}
	out.Lines = all[:min(len(all), FilePreviewLines)]
	return out, nil
}

// countNewlines counts the newlines left in r, streaming, and returns the last byte read.
func countNewlines(r io.Reader) (n int, last byte, err error) {
	buf := make([]byte, 64<<10)
	for {
		k, rerr := r.Read(buf)
		if k > 0 {
			n += bytes.Count(buf[:k], []byte{'\n'})
			last = buf[k-1]
		}
		if errors.Is(rerr, io.EOF) {
			return n, last, nil
		}
		if rerr != nil {
			return n, last, rerr
		}
	}
}

func tablePreview(name string, r io.Reader) (FilePreview, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	if strings.EqualFold(path.Ext(name), ".tsv") {
		cr.Comma = '\t'
	}
	out := FilePreview{Kind: "table"}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A malformed row ends the count; what was read still previews.
			out.Truncated = true
			break
		}
		if len(out.Rows) < FilePreviewRows {
			out.Rows = append(out.Rows, rec)
		}
		out.Total++
	}
	out.Truncated = out.Truncated || out.Total > len(out.Rows)
	return out, nil
}

func zipPreview(r io.ReaderAt, size int64) (FilePreview, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return FilePreview{Kind: "none"}, nil // not a readable zip: stored, nothing to list
	}
	out := FilePreview{Kind: "zip"}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		out.Total++
		if len(out.Entries) < FilePreviewEntries {
			out.Entries = append(out.Entries, FileEntry{Name: f.Name, Size: int64(f.UncompressedSize64)})
		}
	}
	out.Truncated = out.Total > len(out.Entries)
	return out, nil
}

// FileDetail is the tile's one line under the name: "18 files", "112 lines",
// "1,204 rows · 6 columns". Empty when there's nothing to say beyond the size.
func FileDetail(p FilePreview) string {
	switch p.Kind {
	case "zip":
		return plural(p.Total, "file")
	case "text":
		if p.Total > 0 {
			return plural(p.Total, "line")
		}
	case "table":
		cols := 0
		for _, r := range p.Rows {
			cols = max(cols, len(r))
		}
		return fmt.Sprintf("%s · %s", plural(p.Total, "row"), plural(cols, "column"))
	}
	return ""
}

func plural(n int, noun string) string {
	s := commas(n) + " " + noun
	if n != 1 {
		s += "s"
	}
	return s
}

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// ExportKey addresses an export by its content and source, so saving the same
// bytes of the same source again finds the copy already made.
func ExportKey(ws, user, source uuid.UUID, data []byte) string {
	return fmt.Sprintf("library-exports/%s/%s/%s-%x", ws, user, source, sha256.Sum256(data))
}

// CountWords is "412 words" for the text, the line an exported document shows.
func CountWords(text string) string {
	n := 0
	for _, w := range strings.Fields(text) {
		// Markdown marks (#, -, >, ---) stand alone and aren't words.
		if strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			n++
		}
	}
	return plural(n, "word")
}

// CountOf is "14 slides" for n of noun.
func CountOf(n int, noun string) string { return plural(n, noun) }
