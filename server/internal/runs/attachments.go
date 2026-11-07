package runs

import (
	"bytes"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/liamphmurphy/lazyagents/server/internal/apperr"
)

// Chat attachments (LAZYAGENTS-72): files a person uploads in the assistant
// conversation. The extension decides the stored type; the bytes must agree.
const (
	MaxChatAttachmentBytes = 25 << 20
	// MaxChatAttachments matches the contract's maxItems for attachment_ids.
	MaxChatAttachments = 5
)

// chatAttachmentTypes maps a supported extension to the stored content type.
var chatAttachmentTypes = map[string]string{
	".pdf": "application/pdf", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".txt": "text/plain; charset=utf-8", ".md": "text/markdown; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8", ".csv": "text/csv; charset=utf-8", ".json": "application/json",
}

// ChatAttachmentName strips any directory part and control characters from an
// uploaded file's name and bounds it to 255 runes.
func ChatAttachmentName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" || name == ".." {
		name = ""
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[len(r)-255:])
	}
	return name
}

// ChatAttachmentType validates an upload by its name, size and first bytes
// and returns the content type to store. Errors are 422s on field file.
func ChatAttachmentType(name string, size int64, head []byte) (string, error) {
	if name == "" {
		return "", apperr.Invalid("file", "the file needs a name")
	}
	if size <= 0 {
		return "", apperr.Invalid("file", "the file is empty")
	}
	if size > MaxChatAttachmentBytes {
		return "", apperr.Invalid("file", "files are limited to 25 MB")
	}
	ext := strings.ToLower(path.Ext(name))
	ct, ok := chatAttachmentTypes[ext]
	if !ok {
		return "", apperr.Invalid("file", "%s isn't supported: attach PDF, images (PNG, JPEG, GIF, WebP), text, Markdown, CSV or JSON", name)
	}
	switch {
	case strings.HasPrefix(ct, "text/") || ct == "application/json":
		// Text must be UTF-8 text: reject binaries renamed to .txt or .csv.
		// (Not the sniffer: it calls Markdown starting with <p> or <!-- HTML.)
		if !isText(head) {
			return "", apperr.Invalid("file", "%s isn't a text file", name)
		}
	case http.DetectContentType(head) != ct:
		return "", apperr.Invalid("file", "%s doesn't look like a %s file", name, strings.TrimPrefix(ext, "."))
	}
	return ct, nil
}

// isText reports UTF-8 without NUL bytes. head may end mid-rune where the
// sniffed prefix was cut, so up to utf8.UTFMax-1 trailing bytes may be partial.
func isText(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	for cut := 0; cut < utf8.UTFMax && cut <= len(head); cut++ {
		b := head[:len(head)-cut]
		if utf8.Valid(b) {
			// Only a final rune that's merely incomplete may be dropped.
			return cut == 0 || !utf8.FullRune(head[len(b):])
		}
	}
	return false
}

// ChatAttachmentDiskName bounds an attachment's name for its workspace file:
// at most 200 bytes, keeping the extension, cut on a rune boundary.
func ChatAttachmentDiskName(name string) string {
	const max = 200
	if len(name) <= max {
		return name
	}
	ext := path.Ext(name)
	if len(ext) > 20 {
		ext = ""
	}
	stem := name[:len(name)-len(ext)]
	n := max - len(ext)
	for n > 0 && !utf8.RuneStart(stem[n]) {
		n--
	}
	return stem[:n] + ext
}
