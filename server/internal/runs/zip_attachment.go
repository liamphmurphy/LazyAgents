package runs

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// Zip attachments: a person can attach an archive to a chat message and the
// assistant reads the files inside. The server unpacks it (the assistant never
// runs unzip on untrusted bytes) under limits that keep a small upload from
// filling the workspace or the runner's memory.
const (
	// MaxZipFiles is how many files of one archive reach the assistant.
	MaxZipFiles = 200
	// MaxZipFileBytes bounds one unpacked file.
	MaxZipFileBytes = 5 << 20
	// MaxZipTotalBytes bounds everything unpacked from one archive.
	MaxZipTotalBytes = 50 << 20
	// maxZipSkipped bounds the skipped files an archive reports by name.
	maxZipSkipped = 20
)

// ZipFile is one unpacked file: Name is a clean relative path using "/".
type ZipFile struct {
	Name string
	Body []byte
}

// ZipSkip is a file left out of the unpacked archive, and why.
type ZipSkip struct {
	Name   string
	Reason string
}

// ZipContents is what ReadZip found. More counts skipped files beyond the
// first maxZipSkipped, which are not named.
type ZipContents struct {
	Files   []ZipFile
	Skipped []ZipSkip
	More    int
}

// ChatAttachmentZipType is the stored content type of a zip upload.
const ChatAttachmentZipType = "application/zip"

// IsZipName reports whether name has the .zip extension.
func IsZipName(name string) bool {
	return strings.EqualFold(path.Ext(name), ".zip")
}

// ValidateZip checks an upload really is a zip with at least one file, so a
// person hears about a bad archive when they attach it rather than after they
// send. It reads only the central directory.
func ValidateZip(r io.ReaderAt, size int64) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return apperr.Invalid("file", "that isn't a readable zip file")
	}
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			return nil
		}
	}
	return apperr.Invalid("file", "the zip has no files in it")
}

// ReadZip unpacks a zip held in memory. Directories and macOS metadata are
// dropped silently; anything else it can't deliver (unsafe path, encrypted,
// not a regular file, over a limit) is reported in Skipped. It errors only
// when body isn't a zip at all.
func ReadZip(body []byte) (ZipContents, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return ZipContents{}, err
	}
	var out ZipContents
	var total int64
	seen := map[string]bool{}
	skip := func(name, reason string) {
		if len(out.Skipped) < maxZipSkipped {
			out.Skipped = append(out.Skipped, ZipSkip{Name: name, Reason: reason})
		} else {
			out.More++
		}
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || isArchiveMetadata(f.Name) {
			continue
		}
		name, ok := zipEntryName(f.Name)
		switch {
		case !ok:
			skip(strings.ToValidUTF8(f.Name, "?"), "unsafe path")
			continue
		case seen[name]:
			skip(name, "duplicate name")
			continue
		case !f.Mode().IsRegular():
			skip(name, "not a regular file")
			continue
		case f.Flags&0x1 != 0:
			skip(name, "encrypted")
			continue
		case len(out.Files) >= MaxZipFiles:
			skip(name, "more than 200 files")
			continue
		case f.UncompressedSize64 > MaxZipFileBytes:
			skip(name, "over 5 MB")
			continue
		case total+int64(f.UncompressedSize64) > MaxZipTotalBytes:
			skip(name, "over the 50 MB total")
			continue
		}
		data, err := readZipFile(f)
		switch {
		case errors.Is(err, errZipFileTooBig):
			skip(name, "over 5 MB")
			continue
		case err != nil:
			skip(name, "couldn't be read")
			continue
		case total+int64(len(data)) > MaxZipTotalBytes:
			skip(name, "over the 50 MB total")
			continue
		}
		seen[name] = true
		total += int64(len(data))
		out.Files = append(out.Files, ZipFile{Name: name, Body: data})
	}
	return out, nil
}

var errZipFileTooBig = errors.New("zip file too big")

// readZipFile reads one entry, trusting the bytes rather than the size its
// header declares.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, MaxZipFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxZipFileBytes {
		return nil, errZipFileTooBig
	}
	return data, nil
}

// isArchiveMetadata is the clutter macOS adds to archives.
func isArchiveMetadata(name string) bool {
	return strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store"
}

// zipEntryName cleans an entry's path to a relative one that stays inside the
// folder it's unpacked into, with each part short enough for a file name. It
// refuses absolute paths, "..", drive letters and control characters.
func zipEntryName(raw string) (string, bool) {
	name := strings.ReplaceAll(raw, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsFunc(name, unicode.IsControl) {
		return "", false
	}
	var parts []string
	for _, p := range strings.Split(name, "/") {
		switch p {
		case "", ".":
			continue
		case "..":
			return "", false
		}
		if strings.HasSuffix(p, ":") { // a drive letter, as in C:\
			return "", false
		}
		parts = append(parts, ChatAttachmentDiskName(p))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "/"), true
}
