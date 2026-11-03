package harness

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/liamphmurphy/lazyagents/server/internal/agentdef"
)

// Skills reach an agent as files in the workspace. Claude Code loads
// .claude/skills/<name>/SKILL.md natively (the description decides when);
// Codex loads .agents/skills/<name>/SKILL.md natively; other harnesses
// get the same file under .lazyagents/skills/ plus an index
// in AGENTS.md telling them when to read it.
const (
	claudeSkillsDir = ".claude/skills"
	plainSkillsDir  = ".lazyagents/skills"
	// SkillsExcludeFile lists workspace paths the turn wrapper adds to
	// .git/info/exclude so skills are never committed.
	SkillsExcludeFile = ".lazyagents/skills-exclude.txt"
)

// RenderSkill is a SKILL.md: YAML frontmatter, then the body.
func RenderSkill(s agentdef.Skill) []byte {
	name, _ := json.Marshal(s.Name) // JSON strings are valid YAML
	desc, _ := json.Marshal(s.Description)
	return []byte(fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, desc, s.Body))
}

// SkillFiles returns the workspace files for a resolved definition's skills.
func SkillFiles(skills []agentdef.Skill, native bool) map[string][]byte {
	files := map[string][]byte{}
	if len(skills) == 0 {
		return files
	}
	dir := plainSkillsDir
	if native {
		dir = claudeSkillsDir
	}
	var exclude []string
	for _, s := range skills {
		files[dir+"/"+s.Name+"/SKILL.md"] = RenderSkill(s)
		if native {
			exclude = append(exclude, "/"+dir+"/"+s.Name+"/")
		}
	}
	if native {
		sort.Strings(exclude)
		files[SkillsExcludeFile] = []byte(strings.Join(exclude, "\n") + "\n")
	}
	return files
}

// skillsIndex is the AGENTS.md section for harnesses without native skills.
func skillsIndex(skills []agentdef.Skill) string {
	var b strings.Builder
	b.WriteString("## Skills\n\nThese are playbooks for specific situations. When a task matches a description, read the file and follow it before you start.\n\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- `%s/%s/SKILL.md`: %s\n", plainSkillsDir, s.Name, s.Description)
	}
	b.WriteString("\n")
	return b.String()
}

// SkillFilesFor installs native skills for the selected harness.
func SkillFilesFor(skills []agentdef.Skill, name string) map[string][]byte {
	files := SkillFiles(skills, true)
	if name != agentdef.HarnessCodex {
		return files
	}
	out := map[string][]byte{}
	for p, body := range files {
		if p == SkillsExcludeFile {
			body = []byte(strings.ReplaceAll(string(body), "/.claude/skills/", "/.agents/skills/"))
		}
		out[strings.ReplaceAll(p, ".claude/skills", ".agents/skills")] = body
	}
	return out
}
