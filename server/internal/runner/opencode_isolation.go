package runner

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// State is a sibling of the checkout, never a child of it. Worktrees get
// independent namespaces without allowing a relative directory to escape.
func stateKey(dir string) string {
	if dir == "" {
		return "main"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(dir)))
}

func (l *Local) OpenCodeStateDir(runID, dir string) string {
	return filepath.Join(filepath.Dir(l.WorkspaceDir(runID)), ".lazyagents", "opencode", stateKey(dir))
}
func (d *Docker) OpenCodeStateDir(_ string, dir string) string {
	return "/.lazyagents/opencode/" + stateKey(dir)
}
func isOpenCode(argv []string) bool {
	for _, a := range argv {
		if harnessPrograms[a] {
			return a == "opencode"
		}
	}
	return false
}

// The operator must still enforce OS isolation: these checks cannot stop a
// concurrent same-user shell. No boolean or path relocation creates a sandbox.
const openCodeGuard = `set -eu
fail() { echo "runner: unsafe OpenCode config: $1" >&2; exit 78; }
p=$(pwd -P)
while :; do
 for f in opencode.json opencode.jsonc .opencode/opencode.json .opencode/opencode.jsonc; do
  if [ -e "$p/$f" ] || [ -L "$p/$f" ]; then fail "$p/$f"; fi
 done
 [ "$p" = / ] && break
 p=$(dirname "$p")
done
# Refuse the previous checkout-local state instead of silently losing a session.
if [ -e .lazyagents/opencode ] || [ -L .lazyagents/opencode ]; then
 fail "legacy checkout-local state; start a new run after removing the old state"
fi
for p in "$HOME/.config/opencode" "$HOME/.opencode" "$XDG_CONFIG_HOME/opencode" /etc/opencode; do
 for f in config.json config.jsonc opencode.json opencode.jsonc; do
  if [ -e "$p/$f" ] || [ -L "$p/$f" ]; then fail "$p/$f"; fi
 done
done
exec "$@"
`

func openCodeEnv(spec ExecSpec, state string) ExecSpec {
	env := make(map[string]string, len(spec.Env)+6)
	// Never pass arbitrary OpenCode config/plugin switches from another layer.
	for k, v := range spec.Env {
		if !strings.HasPrefix(k, "OPENCODE_") {
			env[k] = v
		}
	}
	env["OPENCODE_CONFIG"] = filepath.Join(state, "opencode.json")
	env["HOME"] = filepath.Join(state, "home")
	for k, sub := range map[string]string{"XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "XDG_STATE_HOME": "state"} {
		env[k] = filepath.Join(state, sub)
	}
	spec.Env = env
	spec.Argv = append([]string{"sh", "-c", openCodeGuard, "lazyagents-opencode"}, spec.Argv...)
	return spec
}

// Reject symlinks before staging, including dangling links and links which
// happen to stay inside the root. This protects trusted destinations across
// turns. OpenRoot additionally prevents a path race from escaping the root.
func noSymlinks(root, rel string) error {
	p := filepath.Join(root, filepath.FromSlash(rel))
	if p != filepath.Clean(root) && !strings.HasPrefix(p, filepath.Clean(root)+string(filepath.Separator)) {
		return fmt.Errorf("runner: path %q escapes root", rel)
	}
	for {
		fi, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runner: symlink in protected path %s", p)
		}
		if p == filepath.Clean(root) {
			return nil
		}
		p = filepath.Dir(p)
	}
}

func (l *Local) prepareOpenCode(runID, dir string, spec ExecSpec) (ExecSpec, error) {
	if !l.OpenCodeIsolated {
		return spec, fmt.Errorf("runner: OpenCode requires OS-isolated config")
	}
	root := filepath.Dir(l.WorkspaceDir(runID))
	if err := noSymlinks(root, "workspace"); err != nil {
		return spec, err
	}
	if rel, err := filepath.Rel(root, dir); err != nil {
		return spec, err
	} else if err := noSymlinks(root, rel); err != nil {
		return spec, err
	}
	state := l.OpenCodeStateDir(runID, spec.Dir)
	if slices.Contains(spec.Argv, "--session") {
		if b, err := os.ReadFile(filepath.Join(state, "initialized")); err != nil || string(b) != "v1" {
			return spec, fmt.Errorf("runner: OpenCode resume state is missing or legacy; start a new run")
		}
	}
	rel, _ := filepath.Rel(root, state)
	if err := noSymlinks(root, rel); err != nil {
		return spec, err
	}
	for name := range spec.Files {
		if err := noSymlinks(dir, name); err != nil {
			return spec, err
		}
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		return spec, err
	}
	for _, name := range []string{"home", "config", "data", "cache", "state"} {
		if err := noSymlinks(state, name); err != nil {
			return spec, err
		}
		if err := os.MkdirAll(filepath.Join(state, name), 0o700); err != nil {
			return spec, err
		}
	}
	for name := range spec.StateFiles {
		if err := noSymlinks(state, name); err != nil {
			return spec, err
		}
	}
	if len(spec.StateFiles) > 0 {
		if err := noSymlinks(state, "initialized"); err != nil {
			return spec, err
		}
		if err := writeFiles(state, map[string][]byte{"initialized": []byte("v1")}); err != nil {
			return spec, err
		}
	}
	if err := writeFiles(state, spec.StateFiles); err != nil {
		return spec, err
	}
	return openCodeEnv(spec, state), nil
}

// Docker stages only after mounting volumes, without following links left by
// an earlier turn. The payload contains generated files, never provider keys.
const openCodeStage = `import os,sys,json,base64,stat
with open(sys.argv[1]) as f: p=json.load(f)
def directory(path):
    if not path.startswith("/"): raise RuntimeError("non-absolute staging root")
    cur="/"
    for part in path.split("/")[1:]:
        if not part: continue
        cur=os.path.join(cur,part)
        try: os.mkdir(cur,0o700)
        except FileExistsError: pass
        if not stat.S_ISDIR(os.lstat(cur).st_mode): raise RuntimeError("unsafe directory: "+cur)
def stage(root,files):
    directory(root)
    for name,data in files.items():
        if name.startswith("/") or ".." in name.split("/"): raise RuntimeError("unsafe file name")
        target=os.path.join(root,name)
        directory(os.path.dirname(target))
        fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|os.O_NOFOLLOW,0o600)
        with os.fdopen(fd,"wb") as f: f.write(base64.b64decode(data))
if p["resume"]:
    try:
        with open(p["state"]+"/initialized") as f: valid=f.read()=="v1"
    except FileNotFoundError: valid=False
    if not valid: raise RuntimeError("OpenCode resume state is missing or legacy; start a new run")
stage(os.getcwd(),p["files"] or {})
for sub in ["home","config","data","cache","state"]: directory(p["state"]+"/"+sub)
stage(p["state"],p["state_files"] or {})
if p["state_files"]: stage(p["state"],{"initialized":base64.b64encode(b"v1").decode()})
# Image-level config switches must not override the runner's selected config.
for k in list(os.environ):
    if k.startswith("OPENCODE_"): del os.environ[k]
os.environ["OPENCODE_CONFIG"]=p["state"]+"/opencode.json"
os.execvp(sys.argv[2],sys.argv[2:])
`
