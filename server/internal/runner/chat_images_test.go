package runner

import (
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/harness"
)

// The moderation classifier tells refusals from other failures, and keeps the
// detail short, on one line and free of secrets.
func TestClassifyImageFailure(t *testing.T) {
	sensitive := map[string]string{"KEY": "sk-secret-123456"}
	for _, tc := range []struct {
		res  harness.Result
		want bool
	}{
		{harness.Result{Error: "Your request was rejected as a result of our safety system."}, true},
		{harness.Result{Error: "moderation_blocked: image prompt"}, true},
		{harness.Result{Final: "I can't help with that request because it violates the content policy."}, true},
		{harness.Result{Final: "Sorry, I cannot create that: it goes against our usage policies."}, true},
		{harness.Result{Final: "I'm unable to generate this image as it would violate OpenAI's policies."}, true},
		{harness.Result{Error: "stream disconnected before completion"}, false},
		{harness.Result{Final: "Image step done."}, false},
		{harness.Result{Final: "I couldn't find the generated file to copy."}, false},
		{harness.Result{}, false},
	} {
		if got, _ := classifyImageFailure(tc.res, sensitive); got != tc.want {
			t.Errorf("%+v: moderation %v, want %v", tc.res, got, tc.want)
		}
	}
	_, detail := classifyImageFailure(harness.Result{Final: "Blocked by moderation.\nkey sk-secret-123456 " + strings.Repeat("x", 500)}, sensitive)
	if strings.Contains(detail, "sk-secret") || strings.Contains(detail, "\n") || len([]rune(detail)) != maxImageReasonRunes || !strings.HasSuffix(detail, "…") {
		t.Fatalf("detail %q", detail)
	}
}

// The person-visible message says whether moderation refused the image.
func TestImageFailureMessage(t *testing.T) {
	for _, tc := range []struct{ kind, reason, want string }{
		{imageFailError, "timed out", "Couldn't create the image: timed out."},
		{imageFailModeration, "rejected by moderation", "The image request was rejected by moderation."},
		{imageFailModeration, "rejected by moderation: it isn't allowed.", "The image request was rejected by moderation: it isn't allowed."},
	} {
		if got := imageFailureMessage(tc.kind, tc.reason); got != tc.want {
			t.Errorf("%s %q: %q, want %q", tc.kind, tc.reason, got, tc.want)
		}
	}
}
