package design

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// The Dusk token snapshot of web/styles/tokens.css, written by the web's
// `npm run gen:design-tokens` alongside web/lib/designs/tokens.generated.json.
// Only its hash is needed here: whether a design's tokens.css set is stale.
//
//go:embed tokens.generated.json
var tokensGenerated []byte

// TokensSource is the tokens_ref.source of a set synced from tokens.css.
const TokensSource = "web/styles/tokens.css"

// TokensHash is sha256 of the tokens.css this build was made from.
var TokensHash = func() string {
	var s struct {
		Hash string `json:"hash"`
	}
	_ = json.Unmarshal(tokensGenerated, &s)
	return s.Hash
}()

// Component is one main component and how many instances it has.
type Component struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Page      string   `json:"page"`
	Instances []string `json:"instances"`
}

// Components lists a design's main components with their instances' ids.
func Components(d Doc) []Component {
	var out []Component
	inst := map[string][]string{}
	Walk(d, func(n, _, page map[string]any, _ int) {
		switch n["type"] {
		case "component":
			name, _ := n["name"].(string)
			if name == "" {
				name = "Component"
			}
			out = append(out, Component{ID: field(n, "id"), Name: name, Page: field(page, "id")})
		case "instance":
			inst[field(n, "component")] = append(inst[field(n, "component")], field(n, "id"))
		}
	})
	for i := range out {
		out[i].Instances = inst[out[i].ID]
		if out[i].Instances == nil {
			out[i].Instances = []string{}
		}
	}
	if out == nil {
		out = []Component{}
	}
	return out
}

// Tokens is design_tokens' answer: the design's token sets and the one in use,
// and whether tokens.css has changed since that set was synced (the editor's
// "tokens.css changed, sync?" badge). Only the person can sync.
type Tokens struct {
	Sets            map[string]any `json:"token_sets"`
	Ref             map[string]any `json:"tokens_ref"`
	TokensCSSChange bool           `json:"tokens_css_changed"`
	CurrentHash     string         `json:"tokens_css_hash"`
	Used            []string       `json:"used"`
}

// TokensOf reads a design's tokens.
func TokensOf(d Doc) Tokens {
	sets, _ := d["token_sets"].(map[string]any)
	if sets == nil {
		sets = map[string]any{}
	}
	ref, _ := d["tokens_ref"].(map[string]any)
	out := Tokens{Sets: sets, Ref: ref, CurrentHash: TokensHash}
	if ref != nil && ref["source"] == TokensSource && ref["hash"] != TokensHash {
		out.TokensCSSChange = true
	}
	used := map[string]bool{}
	var scan func(v any)
	scan = func(v any) {
		switch x := v.(type) {
		case string:
			if len(x) > 1 && x[0] == '$' && reTokenName.MatchString(x[1:]) {
				used[x[1:]] = true
			}
		case map[string]any:
			for k, e := range x {
				switch k {
				case "children", "text", "label", "name", "alt", "id", "d", "author", "linked_tickets", "linked_paths":
					// Not colours (text can start with "$").
				default:
					scan(e)
				}
			}
		case []any:
			for _, e := range x {
				scan(e)
			}
		}
	}
	for _, p := range pages(d) {
		scan(p["background"])
	}
	Walk(d, func(n, _, _ map[string]any, _ int) { scan(n) })
	out.Used = make([]string, 0, len(used))
	for k := range used {
		out.Used = append(out.Used, k)
	}
	sort.Strings(out.Used)
	return out
}
