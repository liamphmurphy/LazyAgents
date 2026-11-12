package design

import (
	"encoding/json"
	"strings"
	"testing"
)

func acceptanceDoc(t *testing.T) Doc {
	t.Helper()
	out, err := Normalize(read(t, fixtureDir+"/acceptance-design.json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFlowsOfTheAcceptanceDesign(t *testing.T) {
	f, err := FlowsOf(acceptanceDoc(t), "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range f.Frames {
		names = append(names, s.ID+"="+s.Name+"@"+s.Page)
	}
	if got := strings.Join(names, " "); got != "n_cart=Cart@p_flows n_ship=Shipping@p_flows n_pay=Payment@p_flows n_done=Confirmation@p_flows" {
		t.Fatalf("frames: %s", got)
	}
	if len(f.Links) != 3 || len(f.Broken) != 0 {
		t.Fatalf("links: %+v broken %v", f.Links, f.Broken)
	}
	l := f.Links[2]
	if l.ID != "k_pay_done" || l.From != "n_pay_btn" || l.Screen != "n_pay" || l.To != "n_done" || l.Trigger != "click" || l.Transition != "smart-animate" || *l.DurationMS != 300 {
		t.Fatalf("link: %+v", l)
	}
	want := `flowchart LR
  n_cart["Cart"]
  n_ship["Shipping"]
  n_pay["Payment"]
  n_done["Confirmation"]
  n_cart -->|"Button / Primary: click · slide-left"| n_ship
  n_ship -->|"Button / Primary: click · slide-left"| n_pay
  n_pay -->|"Button / Primary: click · smart-animate"| n_done`
	if f.Mermaid != want {
		t.Fatalf("mermaid:\n%s", f.Mermaid)
	}
	// The JSON the tool returns.
	b, _ := json.Marshal(f)
	if !strings.Contains(string(b), `"frames":[{"id":"n_cart","name":"Cart","page":"p_flows","type":"frame"}`) || strings.Contains(string(b), "broken_links") {
		t.Fatalf("json: %s", b)
	}
}

func TestFlowsByPage(t *testing.T) {
	d := acceptanceDoc(t)
	f, err := FlowsOf(d, "Diagrams")
	if err != nil || len(f.Frames) != 0 || len(f.Links) != 0 || f.Mermaid != "flowchart LR" {
		t.Fatalf("%+v %v", f, err)
	}
	if f, err = FlowsOf(d, "p_flows"); err != nil || len(f.Links) != 3 {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := FlowsOf(d, "Nowhere"); err == nil {
		t.Fatal("an unknown page should be refused")
	}
}

// A link whose target frame was removed is valid but broken: it's reported,
// and left out of the graph.
func TestFlowsSkipBrokenLinks(t *testing.T) {
	body := strings.Replace(read(t, fixtureDir+"/acceptance-design.json"), `"to": "n_done"`, `"to": "n_gone"`, 1)
	out, err := Normalize(body)
	if err != nil {
		t.Fatalf("a link to a removed frame should normalise: %v", err)
	}
	d, _ := Parse(out)
	f, err := FlowsOf(d, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Links) != 2 || len(f.Broken) != 1 || f.Broken[0] != "k_pay_done" || strings.Contains(f.Mermaid, "n_gone") {
		t.Fatalf("%+v", f)
	}
}

func TestMermaidQuotesLabels(t *testing.T) {
	if got := mermaidText("Say \"hi\"\nnow"); got != `"Say #quot;hi#quot; now"` {
		t.Fatal(got)
	}
}
