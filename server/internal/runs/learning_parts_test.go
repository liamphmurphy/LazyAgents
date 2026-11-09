package runs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

func twoParts() []CoursePart {
	return []CoursePart{{Title: "Foundations", Summary: "s", Minutes: 300}, {Title: "Practice", Summary: "s", Minutes: 300}}
}

func courseWith(t *testing.T, state string, part, budget int32, o CourseOutline) store.LearningCourse {
	t.Helper()
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	return store.LearningCourse{State: state, Part: part, MinutesBudget: budget, Outline: raw}
}

func TestOutlineCapsArePerPart(t *testing.T) {
	var a, b []CourseLesson
	for i := range learningMaxLessons {
		a = append(a, lesson("a"+strings.Repeat("x", i)))
		b = append(b, lesson("b"+strings.Repeat("x", i)))
	}
	o := outline(a, b)
	o.Parts, o.Units[1].Part = twoParts(), 1
	if err := o.Validate(); err != nil {
		t.Fatalf("20 lessons in each part: %v", err)
	}
	o.Units[1].Part = 0
	if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("40 in one part: %v", err)
	}
	o.Units[0].Part, o.Units[1].Part = 1, 0
	if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "wrong part") {
		t.Fatalf("parts out of order: %v", err)
	}
	o = outline([]CourseLesson{lesson("a")})
	o.Parts = twoParts()[:1]
	if err := o.Validate(); err == nil || !strings.Contains(err.Error(), "parts") {
		t.Fatalf("one part: %v", err)
	}
}

func TestMergePartOutline(t *testing.T) {
	first := outline([]CourseLesson{lesson("a")})
	first.Parts = twoParts()

	// A long module's first outline must plan its parts; a short one has none.
	if _, err := mergePartOutline(courseWith(t, "scoping", 0, 1200, CourseOutline{}), outline([]CourseLesson{lesson("a")})); err == nil || !strings.Contains(err.Error(), "parts") {
		t.Fatalf("long without parts: %v", err)
	}
	short, err := mergePartOutline(courseWith(t, "scoping", 0, 180, CourseOutline{}), first)
	if err != nil || short.Parts != nil {
		t.Fatalf("short: %+v %v", short.Parts, err)
	}
	long, err := mergePartOutline(courseWith(t, "scoping", 0, 1200, CourseOutline{}), first)
	if err != nil || len(long.Parts) != 2 {
		t.Fatalf("long: %+v %v", long, err)
	}
	long.Glossary = []CourseTerm{{Term: "Tuple", Definition: "d"}}

	// The next part joins what's built: it may build on earlier lessons, but
	// can't reuse their keys, and a resubmission replaces only its own units.
	c := courseWith(t, "scoping", 1, 1200, long)
	next := outline([]CourseLesson{lesson("b", "a")})
	next.Units[0].Title, next.Parts = "Later", []CoursePart{{Title: "ignored"}}
	next.Glossary = []CourseTerm{{Term: "tuple", Definition: "again"}, {Term: "Vacuum", Definition: "d"}}
	merged, err := mergePartOutline(c, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Units) != 2 || merged.Units[1].Part != 1 || merged.Parts[1].Title != "Practice" || len(merged.Glossary) != 2 {
		t.Fatalf("merged: %+v", merged)
	}
	c = courseWith(t, "scoping", 1, 1200, merged)
	again := outline([]CourseLesson{lesson("c", "a")})
	again.Units[0].Title = "Redo"
	if merged, err = mergePartOutline(c, again); err != nil || len(merged.Units) != 2 || merged.Units[1].Title != "Redo" {
		t.Fatalf("resubmitted: %+v %v", merged, err)
	}
	reused := outline([]CourseLesson{lesson("a")})
	reused.Units[0].Title = "Later"
	if _, err := mergePartOutline(c, reused); err == nil || !strings.Contains(err.Error(), "used twice") {
		t.Fatalf("reused key: %v", err)
	}
}

func TestStudyableUnitsAndApplyKeepEarlierParts(t *testing.T) {
	o := outline([]CourseLesson{lesson("a")}, []CourseLesson{lesson("b", "a"), lesson("c")})
	o.Parts, o.Units[1].Part = twoParts(), 1
	for _, tc := range []struct {
		state string
		part  int32
		want  int
	}{{"authoring", 0, 0}, {"ready", 0, 2}, {"outline_ready", 1, 1}, {"scoping", 1, 1}, {"ready", 1, 2}} {
		if got := StudyableUnits(store.LearningCourse{State: tc.state, Part: tc.part}, o); got != tc.want {
			t.Errorf("%s part %d: %d, want %d", tc.state, tc.part, got, tc.want)
		}
	}

	got, err := o.Apply(OutlineEdit{Units: []OutlineEditUnit{{Title: "Unit B", Lessons: []string{"c", "b"}}}, Known: []string{"c"}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Units) != 2 || got.Units[0].Title != "Unit A" || got.Units[1].Part != 1 || got.Units[1].Lessons[0].Key != "c" || !got.Units[1].Lessons[0].Known {
		t.Fatalf("applied: %+v", got)
	}
	// An earlier part's units and lessons can't be touched from a later one.
	if _, err := o.Apply(OutlineEdit{Units: []OutlineEditUnit{{Title: "Unit A", Lessons: []string{"a"}}}}, 1); err == nil {
		t.Fatal("edited an earlier part's unit")
	}
	if _, err := o.Apply(OutlineEdit{Units: []OutlineEditUnit{{Title: "Unit B", Lessons: []string{"b", "c"}}}, Known: []string{"a"}}, 1); err == nil {
		t.Fatal("marked an earlier part's lesson known")
	}
	if got := o.activeUnits(1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("active: %v", got)
	}
}
