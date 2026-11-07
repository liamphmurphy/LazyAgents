"""Validate active hooks before trusting this invocation's generated policy hook.

Only Vesper's exact session hook and already trusted/managed hooks may run.
This is an offline protocol preflight: no thread or model turn is started.
"""
import json
import os
import selectors
import shlex
import subprocess
import sys
import time


def validate_hooks(result, expected):
    found = 0
    for group in result.get("data", []):
        if group.get("errors"):
            raise RuntimeError("Codex hook configuration has errors")
        for hook in group.get("hooks", []):
            if not hook.get("enabled"):
                continue
            if (hook.get("source") == "sessionFlags" and hook.get("handlerType") == "command"
                    and hook.get("command") == expected and hook.get("eventName") == "preToolUse"
                    and hook.get("matcher") == "*" and not hook.get("async", False)):
                found += 1
            elif hook.get("trustStatus") not in ("trusted", "managed"):
                raise RuntimeError("Unreviewed Codex hook found; review it in Codex before running Vesper")
    if found != 1:
        raise RuntimeError("Vesper policy hook was not loaded exactly once; refusing to run without tool gating")


def main():
    program, args = sys.argv[1], sys.argv[2:]
    config = []
    for i, arg in enumerate(args[:-1]):
        if arg == "-c":
            config.extend(["-c", args[i + 1]])
    expected = os.environ["LAZYAGENTS_POLICY_HOOK_COMMAND"]
    hook_path = os.environ["LAZYAGENTS_POLICY_HOOK_PATH"]
    if not os.path.isabs(hook_path) or not os.path.isfile(hook_path):
        raise RuntimeError("Policy hook script is missing or is not absolute: " + hook_path)
    # Exercise the exact generated command from the process's actual cwd. The
    # --check path only reads the run's policy; it starts no model or tool action.
    checked = subprocess.run(shlex.split(expected) + ["--check"], capture_output=True, text=True, timeout=20)
    if checked.returncode != 0:
        raise RuntimeError("Policy hook command could not execute: " + hook_path)
    try:
        decision = json.loads(checked.stdout)
    except ValueError:
        raise RuntimeError("Policy hook command returned invalid JSON")
    if decision != {}:
        reason = decision.get("hookSpecificOutput", {}).get("permissionDecisionReason", "Policy check was denied") if isinstance(decision, dict) else "Policy check returned an invalid response"
        raise RuntimeError("Policy hook preflight failed: " + str(reason))
    p = subprocess.Popen([program, "app-server", *config, "--listen", "stdio://"],
                         stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    sequence = 0
    def send(method, params, request=True):
        nonlocal sequence
        sequence += 1
        message = {"method": method, "params": params}
        if request:
            message["id"] = sequence
        p.stdin.write((json.dumps(message) + "\n").encode())
        p.stdin.flush()
        return sequence
    selector = selectors.DefaultSelector()
    selector.register(p.stdout, selectors.EVENT_READ)
    pending = b""
    def receive(wanted):
        nonlocal pending
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if b"\n" not in pending:
                if not selector.select(max(0, deadline - time.monotonic())):
                    break
                chunk = os.read(p.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError("Codex hook preflight exited unexpectedly")
                pending += chunk
                continue
            line, pending = pending.split(b"\n", 1)
            message = json.loads(line)
            if message.get("id") == wanted:
                if "error" in message:
                    raise RuntimeError("Codex does not support the required hook preflight")
                return message["result"]
        raise RuntimeError("Codex hook preflight timed out")
    try:
        receive(send("initialize", {"clientInfo": {"name": "lazyagents", "version": "1"}, "capabilities": {"experimentalApi": True}}))
        send("initialized", {}, False)
        validate_hooks(receive(send("hooks/list", {"cwds": [os.getcwd()]})), expected)
    finally:
        selector.close()
        p.terminate()
        try:
            p.wait(timeout=5)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()
    # This bypass covers only our vetted hook and hooks already trusted above.
    os.execvp(program, [program, *args])


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        message = "Vesper Codex preflight: " + str(error)
        print(json.dumps({"type": "error", "message": message}), flush=True)
        print(message, file=sys.stderr)
        sys.exit(1)
