package runs

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	body string
	mode fs.FileMode // 0 means a regular file
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			h.SetMode(e.mode)
		}
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func names(fs []ZipFile) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

func TestReadZipKeepsFilesAndNamesWhatItSkips(t *testing.T) {
	long := strings.Repeat("x", MaxZipFileBytes+1)
	body := makeZip(t,
		zipEntry{name: "src/"}, // directory
		zipEntry{name: "src/main.go", body: "package main"},
		zipEntry{name: "README.md", body: "# hi"},
		zipEntry{name: "__MACOSX/._README.md", body: "junk"},
		zipEntry{name: "docs/.DS_Store", body: "junk"},
		zipEntry{name: "../escape.txt", body: "no"},
		zipEntry{name: "/abs.txt", body: "no"},
		zipEntry{name: "a/../../up.txt", body: "no"},
		zipEntry{name: "C:/win.txt", body: "no"},
		zipEntry{name: "win\\path\\file.txt", body: "backslashes become folders"},
		zipEntry{name: "./dot/./file.txt", body: "cleaned"},
		zipEntry{name: "link", body: "src/main.go", mode: fs.ModeSymlink | 0o777},
		zipEntry{name: "big.bin", body: long},
		zipEntry{name: "README.md", body: "again"},
	)
	got, err := ReadZip(body)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/main.go", "README.md", "win/path/file.txt", "dot/file.txt"}
	if g := names(got.Files); strings.Join(g, ",") != strings.Join(want, ",") {
		t.Fatalf("files %v, want %v", g, want)
	}
	if string(got.Files[0].Body) != "package main" {
		t.Errorf("body %q", got.Files[0].Body)
	}
	reasons := map[string]string{}
	for _, s := range got.Skipped {
		reasons[s.Name] = s.Reason
	}
	for name, reason := range map[string]string{
		"../escape.txt": "unsafe path", "/abs.txt": "unsafe path", "a/../../up.txt": "unsafe path", "C:/win.txt": "unsafe path",
		"link": "not a regular file", "big.bin": "over 5 MB", "README.md": "duplicate name",
	} {
		if reasons[name] != reason {
			t.Errorf("%s skipped as %q, want %q (all: %v)", name, reasons[name], reason, reasons)
		}
	}
}

func TestReadZipStopsAtTheFileAndTotalLimits(t *testing.T) {
	var many []zipEntry
	for i := 0; i < MaxZipFiles+3; i++ {
		many = append(many, zipEntry{name: "f" + string(rune('a'+i%26)) + strings.Repeat("_", i/26) + ".txt", body: "x"})
	}
	got, err := ReadZip(makeZip(t, many...))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != MaxZipFiles || len(got.Skipped) != 3 || got.Skipped[0].Reason != "more than 200 files" {
		t.Fatalf("%d files, skipped %v", len(got.Files), got.Skipped)
	}

	chunk := strings.Repeat("a", MaxZipFileBytes)
	var big []zipEntry
	for i := 0; i < MaxZipTotalBytes/MaxZipFileBytes+2; i++ {
		big = append(big, zipEntry{name: string(rune('a'+i)) + ".txt", body: chunk})
	}
	got, err = ReadZip(makeZip(t, big...))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != MaxZipTotalBytes/MaxZipFileBytes || len(got.Skipped) != 2 || got.Skipped[0].Reason != "over the 50 MB total" {
		t.Fatalf("%d files, skipped %v", len(got.Files), got.Skipped)
	}
}

func TestReadZipRejectsWhatIsNotAZip(t *testing.T) {
	if _, err := ReadZip([]byte("PK\x03\x04 but not really")); err == nil {
		t.Fatal("garbage read as a zip")
	}
}

func TestChatUploadTypeTakesZipsOnlyForChat(t *testing.T) {
	head := []byte("PK\x03\x04rest")
	if ct, err := ChatUploadType("Project.ZIP", 10, head); err != nil || ct != ChatAttachmentZipType {
		t.Errorf("zip: %q %v", ct, err)
	}
	for _, c := range []struct {
		name string
		size int64
		head []byte
	}{
		{"a.zip", 10, []byte("%PDF-1.7")},           // renamed PDF
		{"a.zip", 10, []byte("PK\x05\x06")},         // empty archive
		{"a.zip", 0, nil},                           // empty file
		{"a.zip", MaxChatAttachmentBytes + 1, head}, // too big
	} {
		if ct, err := ChatUploadType(c.name, c.size, c.head); err == nil {
			t.Errorf("%s (%d bytes, %q) accepted as %s", c.name, c.size, c.head, ct)
		}
	}
	if ct, err := ChatUploadType("a.txt", 10, []byte("hi")); err != nil || ct != "text/plain; charset=utf-8" {
		t.Errorf("text still works: %q %v", ct, err)
	}
	// Spaces and the library keep refusing archives.
	if _, err := ChatAttachmentType("a.zip", 10, head); err == nil {
		t.Error("ChatAttachmentType accepts a zip")
	}
}

func TestValidateZip(t *testing.T) {
	ok := makeZip(t, zipEntry{name: "a.txt", body: "x"})
	if err := ValidateZip(bytes.NewReader(ok), int64(len(ok))); err != nil {
		t.Errorf("good zip: %v", err)
	}
	onlyDir := makeZip(t, zipEntry{name: "dir/"})
	if err := ValidateZip(bytes.NewReader(onlyDir), int64(len(onlyDir))); err == nil {
		t.Error("a zip of only folders passed")
	}
	trunc := ok[:len(ok)/2]
	if err := ValidateZip(bytes.NewReader(trunc), int64(len(trunc))); err == nil {
		t.Error("a truncated zip passed")
	}
}
