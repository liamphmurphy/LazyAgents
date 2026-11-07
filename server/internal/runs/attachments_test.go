package runs

import (
	"strings"
	"testing"
)

func TestChatAttachmentName(t *testing.T) {
	for in, want := range map[string]string{
		"report.pdf":                      "report.pdf",
		"../../etc/passwd":                "passwd",
		`C:\Users\me\a b.csv`:             "a b.csv",
		" notes\x00.md ":                  "notes.md",
		"..":                              "",
		"dir/":                            "dir",
		strings.Repeat("x", 300) + ".txt": strings.Repeat("x", 251) + ".txt",
	} {
		if got := ChatAttachmentName(in); got != want {
			t.Errorf("ChatAttachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatAttachmentType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	pdf := []byte("%PDF-1.7\n")
	cases := []struct {
		name string
		size int64
		head []byte
		want string // "" means rejected
	}{
		{"a.png", 10, png, "image/png"},
		{"a.PDF", 10, pdf, "application/pdf"},
		{"a.csv", 10, []byte("a,b\n1,2\n"), "text/csv"},
		{"a.md", 10, []byte("# Hi ünïcode"), "text/markdown"},
		{"a.json", 10, []byte(`{"a":1}`), "application/json"},
		{"a.png", 10, pdf, ""},                         // renamed PDF
		{"a.txt", 10, []byte{0, 1, 2, 0xff, 0xfe}, ""}, // binary as text
		{"a.zip", 10, []byte("PK\x03\x04"), ""},
		{"a.svg", 10, []byte("<svg/>"), ""},
		{"a.txt", 0, nil, ""},
		{"a.txt", MaxChatAttachmentBytes + 1, []byte("x"), ""},
		{"", 10, []byte("x"), ""},
	}
	for _, c := range cases {
		got, err := ChatAttachmentType(c.name, c.size, c.head)
		if c.want == "" {
			if err == nil {
				t.Errorf("%s (%d bytes) accepted as %s", c.name, c.size, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}
