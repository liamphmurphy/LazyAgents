package runs

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "contents of %s", n)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func preview(t *testing.T, name string, data []byte) FilePreview {
	t.Helper()
	p, err := BuildFilePreview(name, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileTypeRefusesExecutablesByNameAndByContent(t *testing.T) {
	for _, c := range []struct {
		name string
		head []byte
	}{
		{"setup.exe", []byte("whatever")},
		{"disk.iso", []byte("x")},
		{"app.dmg", []byte("x")},
		{"tool.jar", []byte("x")},
		{"notes.txt", []byte("MZ\x90\x00\x03")},          // a Windows executable renamed
		{"data.bin", []byte("\x7fELF\x02\x01\x01")},      // ELF
		{"photo.png", []byte{0xcf, 0xfa, 0xed, 0xfe, 7}}, // Mach-O
	} {
		if _, err := FileType(c.name, c.head); err == nil {
			t.Errorf("%s was accepted", c.name)
		}
	}
}

func TestFileTypeKeepsCodeDataAndOfficeFiles(t *testing.T) {
	for name, want := range map[string]string{
		"render.py":   "text/plain; charset=utf-8",
		"run.sh":      "text/plain; charset=utf-8", // a script is code, not an installer
		"q3.csv":      "text/csv",
		"a.xlsx":      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"deck.pdf":    "application/pdf",
		"proj.zip":    "application/zip",
		"README":      "text/plain; charset=utf-8",
		"weird.xyz12": "application/octet-stream",
	} {
		head := []byte("print('hi')\n")
		if name == "weird.xyz12" {
			head = []byte{1, 2, 3, 0, 4}
		}
		got, err := FileType(name, head)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", name, got, err, want)
		}
	}
}

// An uploaded .html must never come back as a page: it's described as text and served as a download.
func TestFileTypeNeverCallsAnythingHTML(t *testing.T) {
	got, err := FileType("page.html", []byte("<script>alert(1)</script>"))
	if err != nil || strings.Contains(got, "html") {
		t.Fatalf("got %q, %v", got, err)
	}
	got, _ = FileType("page.bin", []byte("<!DOCTYPE html><html>"))
	if strings.Contains(got, "html") {
		t.Fatalf("sniffed HTML survived: %q", got)
	}
}

func TestZipPreviewListsEntriesWithoutExtracting(t *testing.T) {
	p := preview(t, "proj.zip", zipOf(t, "a.py", "src/b.py", "docs/"))
	if p.Kind != "zip" || p.Total != 2 || len(p.Entries) != 2 || p.Entries[1].Name != "src/b.py" || FileDetail(p) != "2 files" {
		t.Fatalf("got %+v / %q", p, FileDetail(p))
	}
	if p.Entries[0].Size != int64(len("contents of a.py")) {
		t.Fatalf("entry size %d", p.Entries[0].Size)
	}
}

func TestZipPreviewCapsTheListButCountsEverything(t *testing.T) {
	names := make([]string, FilePreviewEntries+30)
	for i := range names {
		names[i] = fmt.Sprintf("f%03d.txt", i)
	}
	p := preview(t, "big.zip", zipOf(t, names...))
	if p.Total != len(names) || len(p.Entries) != FilePreviewEntries || !p.Truncated {
		t.Fatalf("total %d listed %d truncated %v", p.Total, len(p.Entries), p.Truncated)
	}
}

func TestACorruptZipIsStoredWithNothingToList(t *testing.T) {
	p := preview(t, "broken.zip", []byte("PK\x03\x04 not really"))
	if p.Kind != "none" || FileDetail(p) != "" {
		t.Fatalf("got %+v", p)
	}
}

func TestTablePreviewShowsTheFirstRowsAndCountsAll(t *testing.T) {
	var b strings.Builder
	b.WriteString("id,name,amount\n")
	for i := 1; i <= 1204; i++ {
		fmt.Fprintf(&b, "%d,\"Name, %d\",%d.50\n", i, i, i)
	}
	p := preview(t, "payouts.csv", []byte(b.String()))
	if p.Kind != "table" || p.Total != 1205 || len(p.Rows) != FilePreviewRows || !p.Truncated {
		t.Fatalf("got kind %s total %d rows %d truncated %v", p.Kind, p.Total, len(p.Rows), p.Truncated)
	}
	if p.Rows[1][1] != "Name, 1" {
		t.Fatalf("quoted comma split: %v", p.Rows[1])
	}
	if got := FileDetail(p); got != "1,205 rows · 3 columns" {
		t.Fatalf("detail %q", got)
	}
}

func TestTSVAndRaggedRowsStillPreview(t *testing.T) {
	p := preview(t, "t.tsv", []byte("a\tb\n1\t2\t3\n"))
	if p.Kind != "table" || p.Total != 2 || len(p.Rows[1]) != 3 {
		t.Fatalf("got %+v", p)
	}
}

func TestTextPreviewShowsTheFirstLinesAndCountsAll(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 312; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}
	p := preview(t, "render.py", []byte(b.String()))
	if p.Kind != "text" || p.Total != 312 || len(p.Lines) != FilePreviewLines || !p.Truncated || p.Lines[0] != "line 1" {
		t.Fatalf("got kind %s total %d lines %d truncated %v first %q", p.Kind, p.Total, len(p.Lines), p.Truncated, p.Lines[0])
	}
	if FileDetail(p) != "312 lines" {
		t.Fatalf("detail %q", FileDetail(p))
	}
}

// A text file bigger than the preview still reports its true line count.
func TestTextPreviewCountsLinesPastTheReadLimit(t *testing.T) {
	line := strings.Repeat("x", 99) + "\n"
	n := (maxPreviewBytes / 100) + 5000
	p := preview(t, "big.log", []byte(strings.Repeat(line, n)))
	if p.Total != n || !p.Truncated {
		t.Fatalf("total %d, want %d (truncated %v)", p.Total, n, p.Truncated)
	}
}

func TestBinaryAndOfficeFilesHaveNoPreview(t *testing.T) {
	for _, name := range []string{"a.xlsx", "d.pdf", "x.bin"} {
		if p := preview(t, name, []byte{0, 1, 2, 0, 3}); p.Kind != "none" || FileDetail(p) != "" {
			t.Errorf("%s: %+v", name, p)
		}
	}
	if p := preview(t, "empty.txt", nil); p.Kind != "none" && p.Total > 1 {
		t.Errorf("empty file: %+v", p)
	}
}

func TestCountWordsIgnoresMarkdownMarks(t *testing.T) {
	for text, want := range map[string]string{"": "0 words", "# Launch notes\n": "2 words", "- one\n- two\n---\n> three": "3 words", "word": "1 word"} {
		if got := CountWords(text); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}
