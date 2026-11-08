"""Policy hook for a turn with no tools: denies every call, without asking Vesper."""
import json
import sys

if sys.argv[1:] == ["--check"]:
    print("{}")
else:
    sys.stdin.read()
    print(json.dumps({"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "This turn has no tools."}}))
