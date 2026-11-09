package runs

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func lesson(key string, prereqs ...string) CourseLesson {
	return CourseLesson{Key: key, Title: strings.ToUpper(key), Objectives: []string{"Explain " + key}, Prereqs: prereqs, Minutes: 5}
}

func outline(units ...[]CourseLesson) CourseOutline {
	o := CourseOutline{Summary: "s"}
	for i, ls := range units {
		o.Units = append(o.Units, CourseUnit{Title: "Unit " + string(rune('A'+i)), Question: "q", Lessons: ls})
	}
	return o
}

func TestOutlineValidateOrder(t *testing.T) {
	for name, tc := range map[string]struct {
		o    CourseOutline
		want string
	}{
		"ok":         {outline([]CourseLesson{lesson("a")}, []CourseLesson{lesson("b", "a")}), ""},
		"later":      {outline([]CourseLesson{lesson("b", "a"), lesson("a")}), "can't come before"},
		"cycle":      {outline([]CourseLesson{lesson("a", "b"), lesson("b", "a")}), "can't come before"},
		"self":       {outline([]CourseLesson{lesson("a", "a")}), "depend on itself"},
		"unknown":    {outline([]CourseLesson{lesson("a", "z")}), "isn't in the outline"},
		"duplicate":  {outline([]CourseLesson{lesson("a"), lesson("a")}), "used twice"},
		"bad key":    {outline([]CourseLesson{lesson("Not A Slug")}), "valid key"},
		"empty unit": {outline([]CourseLesson{}), "no lessons"},
		"too many":   {outline(nil, nil, nil, nil, nil, nil), "units"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.o.Validate()
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOutlineValidateLessonCap(t *testing.T) {
	var ls []CourseLesson
	for i := 0; i <= learningMaxLessons; i++ {
		ls = append(ls, lesson("l"+strings.Repeat("x", i)))
	}
	o := outline(ls)
	if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("got %v", err)
	}
}

func TestOutlineApply(t *testing.T) {
	o := outline([]CourseLesson{lesson("a"), lesson("b", "a")}, []CourseLesson{lesson("c")})
	units := func(lists ...[]string) []OutlineEditUnit {
		out := []OutlineEditUnit{}
		for i, l := range lists {
			out = append(out, OutlineEditUnit{Title: o.Units[i].Title, Lessons: l})
		}
		return out
	}
	got, err := o.Apply(OutlineEdit{Units: units([]string{"a"}, []string{"c", "b"}), Known: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.find("a").Known || got.find("b").Known || got.Units[1].Lessons[0].Key != "c" {
		t.Fatalf("got %+v", got)
	}
	if _, err := o.Apply(OutlineEdit{Units: units([]string{"b", "a"}, []string{"c"})}); err == nil || !strings.Contains(err.Error(), "can't come before") {
		t.Fatalf("reorder: %v", err)
	}
	if _, err := o.Apply(OutlineEdit{Units: units([]string{"b"}, []string{"c"})}); err == nil {
		t.Fatal("removing a prerequisite must be refused")
	}
	if _, err := o.Apply(OutlineEdit{Units: []OutlineEditUnit{{Title: o.Units[0].Title, Lessons: []string{"new"}}}}); err == nil {
		t.Fatal("an edit can't bring in a new lesson")
	}
}

func TestActiveUnitsSkipAllKnown(t *testing.T) {
	a := lesson("a")
	a.Known = true
	o := outline([]CourseLesson{a}, []CourseLesson{lesson("b")})
	if got := o.activeUnits(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("got %v", got)
	}
}

func validLesson() LessonSubmission {
	return LessonSubmission{Key: "a", Blocks: []LessonBlock{{Kind: "text", Text: "x", Provenance: "sourced", Sources: []int{1}}},
		Checks: []LessonCheckInput{{Objective: "o", Kind: "mcq", Prompt: "p", Options: []string{"x", "y", "z"}, Answer: "x", Explanation: "e"}}}
}

func diagramBlock() LessonBlock {
	return LessonBlock{Kind: "diagram", Text: `<svg viewBox="0 0 9 9"><rect fill="var(--viz-panel)"/></svg>`, Alt: "A box", Provenance: "model_knowledge"}
}

func TestLessonVisualsDontEatTextBudget(t *testing.T) {
	l := validLesson()
	l.Blocks[0].Text = strings.Repeat("x", learningBodyRunes)
	l.Blocks = append(l.Blocks, diagramBlock())
	if err := l.validate([]string{"o"}, map[int]bool{1: true}); err != nil {
		t.Fatal(err)
	}
}

func TestTutorLessonTextDropsMarkup(t *testing.T) {
	got := tutorLessonText([]byte(`[{"kind":"text","text":"keep"},{"kind":"diagram","text":"<svg>huge</svg>","alt":"A box"}]`))
	if !strings.Contains(got, "keep") || !strings.Contains(got, "A box") || strings.Contains(got, "huge") {
		t.Fatal(got)
	}
	if got := tutorLessonText([]byte(`not json`)); got != "not json" {
		t.Fatal(got)
	}
}

func TestLessonValidate(t *testing.T) {
	sources := map[int]bool{1: true}
	for name, tc := range map[string]struct {
		edit func(*LessonSubmission)
		want string
	}{
		"ok":                {func(*LessonSubmission) {}, ""},
		"no provenance":     {func(l *LessonSubmission) { l.Blocks[0].Provenance = "" }, "provenance"},
		"sourced no cites":  {func(l *LessonSubmission) { l.Blocks[0].Sources = nil }, "cites no sources"},
		"invented source":   {func(l *LessonSubmission) { l.Blocks[0].Sources = []int{9} }, "no source 9"},
		"key not an option": {func(l *LessonSubmission) { l.Checks[0].Answer = "q" }, "exactly one"},
		"two options":       {func(l *LessonSubmission) { l.Checks[0].Options = []string{"x", "y"} }, "at least 3"},
		"repeat option":     {func(l *LessonSubmission) { l.Checks[0].Options = []string{"x", "x", "y"} }, "repeated"},
		"wrong objective":   {func(l *LessonSubmission) { l.Checks[0].Objective = "other" }, "word for word"},
		"short no rubric":   {func(l *LessonSubmission) { l.Checks[0].Kind, l.Checks[0].Rubric = "short", "" }, "rubric"},
		"no checks":         {func(l *LessonSubmission) { l.Checks = nil }, "checks"},
		"diagram":           {func(l *LessonSubmission) { l.Blocks = append(l.Blocks, diagramBlock()) }, ""},
		"step-through": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Text, b.Steps = `<svg viewBox="0 0 9 9"><g data-step="2"/></svg>`, []string{"one", "two"}
			l.Blocks = append(l.Blocks, b)
		}, ""},
		"chart": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Kind, b.Text = "chart", `{"type":"bar","categories":["a"],"series":[{"name":"s","values":[1]}]}`
			l.Blocks = append(l.Blocks, b)
		}, ""},
		"diagram no alt":        {func(l *LessonSubmission) { b := diagramBlock(); b.Alt = " "; l.Blocks = append(l.Blocks, b) }, "alt text"},
		"diagram no provenance": {func(l *LessonSubmission) { b := diagramBlock(); b.Provenance = ""; l.Blocks = append(l.Blocks, b) }, "diagram block 1 needs a provenance"},
		"diagram bad source": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Provenance, b.Sources = "sourced", []int{9}
			l.Blocks = append(l.Blocks, b)
		}, "no source 9"},
		"diagram bad svg": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Text = `<svg viewBox="0 0 1 1">`
			l.Blocks = append(l.Blocks, b)
		}, "diagram block 1: the SVG"},
		"one step": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Steps = []string{"only"}
			l.Blocks = append(l.Blocks, b)
		}, "2 to 12 steps"},
		"empty step": {func(l *LessonSubmission) {
			b := diagramBlock()
			b.Text, b.Steps = `<svg viewBox="0 0 9 9"><g data-step="2"/></svg>`, []string{"one", " "}
			l.Blocks = append(l.Blocks, b)
		}, "needs a caption"},
		"too many visuals": {func(l *LessonSubmission) {
			for range learningMaxVisuals + 1 {
				l.Blocks = append(l.Blocks, diagramBlock())
			}
		}, "at most 6"},
		"unknown kind": {func(l *LessonSubmission) { l.Blocks[0].Kind = "video" }, "diagram or chart"},
	} {
		t.Run(name, func(t *testing.T) {
			l := validLesson()
			tc.edit(&l)
			err := l.validate([]string{"o"}, sources)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	l := validLesson()
	if err := l.validate([]string{"o", "uncovered"}, sources); err == nil || !strings.Contains(err.Error(), "no check tests") {
		t.Fatalf("got %v", err)
	}
}

func TestGradeAndReveal(t *testing.T) {
	yes, no := true, false
	mcq := store.LearningCheck{ID: uuid.New(), Kind: "mcq", Options: []string{"a", "b", "c"}, Answer: "b", Explanation: "e"}
	order := store.LearningCheck{ID: uuid.New(), Kind: "order", Options: []string{"1", "2", "3"}, Explanation: "e"}
	short := store.LearningCheck{ID: uuid.New(), Kind: "short", Answer: "m", Rubric: "r", Explanation: "e"}
	if g := gradeCheck(mcq, []string{"b"}); g == nil || !*g {
		t.Fatal("right mcq")
	}
	if g := gradeCheck(mcq, []string{"b", "a"}); g == nil || *g {
		t.Fatal("two choices can't be right")
	}
	if g := gradeCheck(order, []string{"2", "1", "3"}); g == nil || *g {
		t.Fatal("wrong order")
	}
	if g := gradeCheck(short, []string{"x"}); g != nil {
		t.Fatal("a short answer is the person's to mark")
	}
	wrong, right, unmarked := store.LearningAttempt{Correct: &no}, store.LearningAttempt{Correct: &yes}, store.LearningAttempt{}
	if RevealCheck(mcq, nil) != nil || RevealCheck(mcq, []store.LearningAttempt{wrong}) != nil {
		t.Fatal("the key must stay hidden before it's earned")
	}
	if RevealCheck(mcq, []store.LearningAttempt{wrong, wrong}) == nil || RevealCheck(mcq, []store.LearningAttempt{right}) == nil {
		t.Fatal("reveal after two misses or a right answer")
	}
	if RevealCheck(short, nil) != nil || RevealCheck(short, []store.LearningAttempt{unmarked}) == nil {
		t.Fatal("a short answer reveals its rubric after an attempt")
	}
	if got := RevealCheck(order, []store.LearningAttempt{right}).Answer; len(got) != 3 {
		t.Fatalf("order answer: %v", got)
	}
}

func TestCheckOptionsStable(t *testing.T) {
	k := store.LearningCheck{ID: uuid.New(), Options: []string{"a", "b", "c", "d", "e"}}
	first := CheckOptions(k)
	if strings.Join(first, "") != strings.Join(CheckOptions(k), "") {
		t.Fatal("options must keep their order between loads")
	}
	if len(first) != 5 || k.Options[0] != "a" {
		t.Fatal("shuffle must not touch the stored order")
	}
}

func TestLessonProgress(t *testing.T) {
	yes, no := true, false
	a, b := store.LearningCheck{ID: uuid.New()}, store.LearningCheck{ID: uuid.New()}
	checks := []store.LearningCheck{a, b}
	at := func(c ...*bool) []store.LearningAttempt {
		out := []store.LearningAttempt{}
		for _, v := range c {
			out = append(out, store.LearningAttempt{Correct: v})
		}
		return out
	}
	for name, tc := range map[string]struct {
		a, b []store.LearningAttempt
		want string
	}{
		"untouched":   {nil, nil, "new"},
		"half":        {at(&yes), nil, "new"},
		"solid":       {at(&yes), at(&yes), "solid"},
		"recovered":   {at(&no, &yes), at(&yes), "complete"},
		"still wrong": {at(&no), at(&yes), "new"},
		"unmarked":    {at(nil), at(&yes), "new"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := LessonProgress(checks, map[uuid.UUID][]store.LearningAttempt{a.ID: tc.a, b.ID: tc.b}); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
