package runs

import (
	"net/http"
	"path"
	"strings"
	"unicode"

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
	".gif": "image/gif", ".webp": "image/webp", ".txt": "text/plain", ".md": "text/markdown",
	".markdown": "text/markdown", ".csv": "text/csv", ".json": "application/json",
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
	sniffed := http.DetectContentType(head)
	switch {
	case strings.HasPrefix(ct, "text/") || ct == "application/json":
		// Text must be text: reject binaries renamed to .txt or .csv.
		if !strings.HasPrefix(sniffed, "text/plain") {
			return "", apperr.Invalid("file", "%s isn't a text file", name)
		}
	case sniffed != ct:
		return "", apperr.Invalid("file", "%s doesn't look like a %s file", name, strings.TrimPrefix(ext, "."))
	}
	return ct, nil
}
