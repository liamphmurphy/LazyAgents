package runs

import "testing"

func TestSVGFenceRewrite(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"```svg-image\n<svg/>\n```", "```svg\n<svg/>\n```"},
		{"> ~~~~ SVG-Image title=x\n", "> ~~~~ svg title=x\n"},
		{"- ```svg-image\n", "- ```svg\n"},
		{"```svg\n```", "```svg\n```"},
		{"an svg-image fence", "an svg-image fence"},
		{"```svg-images\n```", "```svg-images\n```"},
	} {
		if got := svgFenceRe.ReplaceAllString(tc.in, "${1}${2}svg"); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}
