package artifacts

import (
	"reflect"
	"testing"
)

func TestInlineTagsOutsideCode(t *testing.T) {
	body := "# Heading\n\n#Work/nested (#other) #work/nested `#inlinecode`\n\n    #indent\n\t#tab\n\n````md\n#fence\n```\n#stillfence\n````\n#after\n~~~\n#unclosed"
	got := Tags(Properties{"tags": []string{"Explicit"}}, body)
	want := []string{"after", "explicit", "other", "work/nested"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags: %v, want %v", got, want)
	}
}
