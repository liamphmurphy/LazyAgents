package links

import (
	"regexp"
	"testing"
)

// fenceMarker replaced a regexp; it must agree with it on every line shape.
func TestFenceMarkerMatchesRegexp(t *testing.T) {
	re := regexp.MustCompile("^ {0,3}(```|~~~)")
	for _, line := range []string{"", "```", "```go", "~~~", "~~~ md", " ```", "   ```", "    ```", "\t```", "``", "~~", "text ```",
		"  ~~~~", "`` `", "  ```", "[[x]] ```", "   ~~~x", "     ~~~"} {
		want := ""
		if m := re.FindStringSubmatch(line); m != nil {
			want = m[1]
		}
		if got := fenceMarker(line); got != want {
			t.Errorf("fenceMarker(%q) = %q, regexp says %q", line, got, want)
		}
	}
}

func TestParseCRLFAndTildeFences(t *testing.T) {
	md := "a [[One]]\r\n~~~\r\n[[Skipped]]\r\n```\r\n[[StillSkipped]]\r\n~~~\r\n    ```\r\n[[Two]]\r\n"
	got := Parse(md)
	if len(got) != 2 || got[0].Target != "One" || got[0].Line != 0 || got[1].Target != "Two" || got[1].Line != 7 {
		t.Fatalf("got %+v", got)
	}
}
