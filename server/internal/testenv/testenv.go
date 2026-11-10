// Package testenv is what a repository's tests need to run in a Vesper turn:
// the services to start next to each turn and the Nix packages to install.
// Nobody has to write it down. The runner infers a first recipe from files the
// repository already has (CI workflows, compose files) and adds the tools
// agents install while they work, so the next run starts with them. A person
// can read, replace or forget it; services declared on the project override
// it.
package testenv

import (
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
)

// Recipe is one repository's test environment.
type Recipe struct {
	Services []agentdef.Service `json:"services,omitempty"`
	// NixPackages are installables such as "nixpkgs#protobuf".
	NixPackages []string `json:"nix_packages,omitempty"`
}

// MaxNixPackages bounds what a recipe installs at the start of every run.
const MaxNixPackages = 40

var installableRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+/:-]*#[A-Za-z0-9._+-]+$`)

// Validate checks a recipe by the rules project services follow, and that each
// package is a flake installable (no shell, no paths).
func (r *Recipe) Validate() error {
	if err := agentdef.ValidateServices(r.Services); err != nil {
		return err
	}
	if len(r.NixPackages) > MaxNixPackages {
		return &InvalidError{"at most 40 Nix packages"}
	}
	for _, p := range r.NixPackages {
		if !installableRe.MatchString(p) {
			return &InvalidError{"nix package " + p + " must look like nixpkgs#protobuf"}
		}
	}
	return nil
}

// InvalidError is a recipe that fails Validate.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return "test environment: " + e.Msg }

// Empty reports whether the recipe asks for nothing.
func (r Recipe) Empty() bool { return len(r.Services) == 0 && len(r.NixPackages) == 0 }

// AddNixPackages returns the recipe with pkgs added, sorted, and whether that
// changed anything. Packages are never dropped automatically: a turn that
// didn't use one isn't evidence the tests don't need it.
func (r Recipe) AddNixPackages(pkgs []string) (Recipe, bool) {
	changed := false
	out := r
	out.NixPackages = slices.Clone(r.NixPackages)
	for _, p := range pkgs {
		if installableRe.MatchString(p) && !slices.Contains(out.NixPackages, p) && len(out.NixPackages) < MaxNixPackages {
			out.NixPackages = append(out.NixPackages, p)
			changed = true
		}
	}
	sort.Strings(out.NixPackages)
	return out, changed
}

// NixInstallScript installs the recipe's packages into the run's profile, as
// shell for the workspace setup. A package that fails to install is reported,
// not fatal: the agent can still work, and see why.
func (r Recipe) NixInstallScript() string {
	if len(r.NixPackages) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("if command -v nix >/dev/null 2>&1; then\n")
	b.WriteString("  echo '→ installing the test environment: " + strings.Join(r.NixPackages, " ") + "'\n")
	b.WriteString("  nix profile install")
	for _, p := range r.NixPackages {
		b.WriteString(" '" + p + "'") // installableRe admits no quote
	}
	b.WriteString(" || echo '→ some test environment packages failed to install; agents can install them with nix profile install'\n")
	b.WriteString("fi\n")
	return b.String()
}

// ParseNixManifest lists the installables in a Nix profile's manifest.json,
// in both the list (version 1–2) and map (version 3) layouts. Only flake
// installables are kept: a store path someone added by hand can't be
// reinstalled on another machine.
func ParseNixManifest(b []byte) []string {
	type element struct {
		AttrPath    string `json:"attrPath"`
		OriginalURL string `json:"originalUrl"`
	}
	var m struct {
		Elements json.RawMessage `json:"elements"`
	}
	if json.Unmarshal(b, &m) != nil || len(m.Elements) == 0 {
		return nil
	}
	var elements []element
	if json.Unmarshal(m.Elements, &elements) != nil {
		byName := map[string]element{}
		if json.Unmarshal(m.Elements, &byName) != nil {
			return nil
		}
		for _, e := range byName {
			elements = append(elements, e)
		}
	}
	var out []string
	for _, e := range elements {
		if e.AttrPath == "" || e.OriginalURL == "" {
			continue
		}
		flake := strings.TrimPrefix(e.OriginalURL, "flake:")
		attr := e.AttrPath
		// legacyPackages.<system>.foo and packages.<system>.foo are what
		// nixpkgs#foo resolves to; keep the portable short form.
		for _, prefix := range []string{"legacyPackages.", "packages."} {
			if rest, ok := strings.CutPrefix(attr, prefix); ok {
				if _, name, ok := strings.Cut(rest, "."); ok {
					attr = name
				}
			}
		}
		p := flake + "#" + attr
		if installableRe.MatchString(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// RepoKey names a repository the same way however it's cloned: host and
// path, lower-cased, without the scheme, user or .git
// (git@github.com:Acme/App.git and https://github.com/acme/app are one).
func RepoKey(url string) string {
	u := strings.TrimSpace(url)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	} else if at := strings.Index(u, "@"); at >= 0 && strings.Contains(u[at:], ":") {
		u = strings.Replace(u[at+1:], ":", "/", 1) // scp-like git@host:path
	}
	if at := strings.LastIndex(u, "@"); at >= 0 && at < strings.Index(u+"/", "/") {
		u = u[at+1:]
	}
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	return strings.ToLower(u)
}
