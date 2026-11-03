#!/usr/bin/env python3
"""Attach the skills in examples/skills/ to the starter agents.

    python3 examples/attach-skills.py [--api http://localhost:8000/api/v1] [--dry-run]

Each skill is a folder with a SKILL.md: YAML frontmatter (name, description) and
a markdown body. Skills with the same name on an agent are replaced; any other
skills the agent already has are kept. Re-running changes nothing.
"""
import argparse, json, pathlib, re, sys, urllib.request, urllib.error

HERE = pathlib.Path(__file__).parent / "skills"

ASSIGN = {
    "tech-lead": ["ticket-breakdown", "architecture-decision-records", "technical-risk-review", "api-contract-first"],
    "frontend-engineer": ["ui-design-fidelity", "ui-states-and-interaction", "accessibility-checklist", "react-nextjs-patterns", "frontend-testing"],
    "backend-engineer": ["go-code-style", "go-error-handling", "go-concurrency", "go-testing", "go-http-api", "postgres-pgx-and-migrations", "api-contract-first"],
    "code-reviewer": ["code-review-checklist", "writing-review-feedback", "security-review", "go-review-pitfalls", "ts-react-review-pitfalls"],
    "product-manager": ["prd-and-requirements", "requirements-verification", "wiki-documentation"],
}


def load(name):
    text = (HERE / name / "SKILL.md").read_text()
    m = re.match(r'---\nname: (.+)\ndescription: "(.+)"\n---\n\n?(.*)', text, re.S)
    if not m or m.group(1) != name:
        sys.exit(f"{name}: malformed SKILL.md")
    return {"name": name, "description": m.group(2), "body": m.group(3).strip()}


def call(api, method, path, body=None):
    req = urllib.request.Request(api + path, method=method, data=json.dumps(body).encode() if body else None,
                                 headers={"Content-Type": "application/json"})
    try:
        return json.load(urllib.request.urlopen(req))
    except urllib.error.HTTPError as e:
        sys.exit(f"{method} {path}: {e.code} {e.read().decode()}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--api", default="http://localhost:8000/api/v1")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()
    for handle, names in ASSIGN.items():
        d = call(args.api, "GET", f"/agents/{handle}")
        spec = d["spec"]
        have = {s["name"]: s for s in spec.get("skills") or []}
        added, changed = [], []
        for n in names:
            s = load(n)
            if n not in have:
                added.append(n)
            elif have[n] != s:
                changed.append(n)
            have[n] = s
        if not (added or changed):
            print(f"{handle}: up to date")
            continue
        note = "; ".join(filter(None, ["Added skills: " + ", ".join(added) if added else "", "Updated skills: " + ", ".join(changed) if changed else ""]))
        print(f"{handle}: {note}")
        if args.dry_run:
            continue
        spec["skills"] = list(have.values())
        call(args.api, "PUT", f"/agents/{handle}/definition", {
            "name": d["agent"]["name"], "role": d["agent"]["role"], "specialty": d["specialty"],
            "color_index": d["agent"]["color_index"], "spec": spec, "note": note})


main()
