package runs

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
)

// The Library keeps a person's assistant-chat keepsakes for good: every image
// a persona generates is added when its reply is stored, and an SVG diagram
// when the person saves it.

// MaxLibraryTitleRunes matches library_items.title.
const MaxLibraryTitleRunes = 300

// MaxLibraryDiagramBytes caps one saved SVG; diagrams are text, so far smaller
// than the image limit.
const MaxLibraryDiagramBytes = 1 << 20

const svgNamespace = "http://www.w3.org/2000/svg"

// LibraryImageTitle names a generated image after the chat it came from, so
// it stays recognisable after the chat is gone.
func LibraryImageTitle(chatTitle string) string {
	return libraryTitle(chatTitle, "Generated image")
}

// LibraryDiagramTitle does the same for a saved diagram.
func LibraryDiagramTitle(chatTitle string) string {
	return libraryTitle(chatTitle, "Diagram")
}

// CheckLibraryDiagram returns the bytes to store for an SVG source. Only the
// root element is checked: the Library shows SVGs through <img>, and serves
// them sandboxed, so what's inside can't run.
func CheckLibraryDiagram(source string) ([]byte, error) {
	svg := []byte(strings.TrimSpace(source))
	if len(svg) == 0 {
		return nil, apperr.Invalid("svg", "required")
	}
	if len(svg) > MaxLibraryDiagramBytes {
		return nil, apperr.Invalid("svg", "over the 1 MiB limit")
	}
	if !IsSVGDocument(svg) {
		return nil, apperr.Invalid("svg", "not an SVG document")
	}
	return svg, nil
}

// IsSVGDocument reports whether the first element is an <svg> in SVG's
// namespace. Only the root is checked, for the reason CheckLibraryDiagram
// gives.
func IsSVGDocument(svg []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(svg))
	for {
		tok, err := d.RawToken()
		if err != nil {
			return false
		}
		if el, ok := tok.(xml.StartElement); ok {
			return el.Name.Local == "svg" && hasSVGNamespace(el)
		}
	}
}

// hasSVGNamespace reports a default xmlns of SVG's, without which an <img>
// can't draw the document.
func hasSVGNamespace(el xml.StartElement) bool {
	for _, a := range el.Attr {
		if a.Name.Space == "" && a.Name.Local == "xmlns" && a.Value == svgNamespace {
			return true
		}
	}
	return false
}

// LibraryDiagramKey is the object key for a person's diagram, the same for the
// same content.
func LibraryDiagramKey(ws, user uuid.UUID, svg []byte) string {
	return fmt.Sprintf("library-diagrams/%s/%s/%x", ws, user, sha256.Sum256(svg))
}

func libraryTitle(chatTitle, fallback string) string {
	t := strings.TrimSpace(chatTitle)
	if t == "" {
		return fallback
	}
	if r := []rune(t); len(r) > MaxLibraryTitleRunes {
		t = string(r[:MaxLibraryTitleRunes])
	}
	return t
}
