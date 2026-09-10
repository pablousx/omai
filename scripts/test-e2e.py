#!/usr/bin/env python3
"""Real CLI/daemon integration using two disposable homes and a bare Git remote."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument("binary", type=Path)
args = parser.parse_args()
binary = args.binary.resolve()


def wait_for(description, predicate, timeout=20):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        if predicate():
            return
        time.sleep(0.1)
    raise AssertionError("Timed out: " + description)


def content(path):
    try:
        return path.read_text()
    except FileNotFoundError:
        return ""


with tempfile.TemporaryDirectory(prefix="omai-e2e-") as folder:
    root = Path(folder)
    remote = root / "remote.git"
    subprocess.run(["git", "init", "--bare", "--initial-branch=main", str(remote)],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    machines = []
    running = []
    logs = []
    try:
        for name in ("A", "B"):
            home = root / name
            home.mkdir()
            env = {k: v for k, v in os.environ.items()
                   if not k.startswith(("GIT_", "CODEX_", "CLAUDE_", "OPENCODE_", "XDG_"))}
            env.update(HOME=str(home), XDG_CONFIG_HOME=str(home / ".config"),
                       XDG_DATA_HOME=str(home / ".local/share"),
                       XDG_STATE_HOME=str(home / ".local/state"),
                       XDG_CACHE_HOME=str(home / ".cache"))
            def cli(*cmd, env=env, expected=(0,)):
                result = subprocess.run([str(binary), *cmd], env=env, text=True,
                                        capture_output=True, timeout=30)
                if result.returncode not in expected:
                    raise AssertionError(f"{cmd}: {result.returncode}\n{result.stdout}\n{result.stderr}")
                return result
            cli("setup", "--yes", "--no-service", "--remote", str(remote),
                "--providers", "codex,claude,opencode", "--machine", name)
            source = home / ".config/omai/source"
            state = home / ".local/state/omai"
            cfg = source.parent / "config.json"
            data = json.loads(cfg.read_text())
            data.update(poll_seconds=1, sync_seconds=1)
            cfg.write_text(json.dumps(data))
            machines.append(dict(home=home, env=env, cli=cli, source=source, state=state))
        a, b = machines
        (a["source"] / "instructions.md").write_text("Initial instructions.\n")
        # Sentinel stores must never be read into the source or remote.
        for m in machines:
            secret = m["home"] / ".codex/auth.json"
            secret.parent.mkdir(parents=True, exist_ok=True)
            secret.write_text('{"access_token":"sentinel-must-stay-local"}')
            session = m["home"] / ".claude/projects/history.jsonl"
            session.parent.mkdir(parents=True, exist_ok=True)
            session.write_text('sentinel-private-session\n')
        for m in machines:
            log = tempfile.TemporaryFile(mode="w+")
            logs.append(log)
            proc = subprocess.Popen([str(binary), "daemon", "run"], env=m["env"],
                                    stdout=log, stderr=log)
            running.append(proc)
            m["proc"] = proc
            if m is a:
                wait_for("first automatic push", lambda: content(a["state"] / "last-sync.json"))
        wait_for("automatic machine relay", lambda: "Initial instructions." in content(b["home"] / ".claude/CLAUDE.md"))
        (a["home"] / ".claude/CLAUDE.md").write_text("Provider-side edit.\n")
        wait_for("reverse import and remote relay", lambda: content(b["source"] / "instructions.md") == "Provider-side edit.\n")
        wait_for("both healthy", lambda: all(json.loads(content(m["state"] / "status.json") or "{}").get("health") == "healthy" for m in machines))
        before = subprocess.check_output(["git", "--git-dir=" + str(remote), "rev-list", "--count", "--all"])
        time.sleep(2.2)
        after = subprocess.check_output(["git", "--git-dir=" + str(remote), "rev-list", "--count", "--all"])
        assert before == after, "Feedback loop created Git commits"
        offline = root / "remote-offline.git"
        remote.rename(offline)
        (a["source"] / "instructions.md").write_text("Offline A.\n")
        (b["source"] / "instructions.md").write_text("Offline B.\n")
        wait_for("offline local propagation", lambda: content(a["home"] / ".claude/CLAUDE.md") == "Offline A.\n" and content(b["home"] / ".codex/AGENTS.md") == "Offline B.\n")
        wait_for("offline health", lambda: all(json.loads(content(m["state"] / "status.json") or "{}").get("health") == "offline" for m in machines))
        # Make reconnection order deterministic while retaining both offline commits.
        b["proc"].terminate()
        b["proc"].wait(timeout=25)
        offline.rename(remote)
        wait_for("automatic retry after reconnect", lambda: json.loads(content(a["state"] / "status.json") or "{}").get("health") == "healthy")
        a["proc"].terminate()
        a["proc"].wait(timeout=25)
        b["cli"]("sync", expected=(2,))
        conflicts = json.loads(b["cli"]("conflicts", "--json").stdout)
        assert any(c["key"] == "git/instructions.md" for c in conflicts)
        assert content(b["source"] / "instructions.md") == "Offline B.\n"
        b["cli"]("conflicts", "--show", "git/instructions.md")
        b["cli"]("resolve", "git/instructions.md", "--take", "local")
        a["cli"]("sync")
        assert content(a["source"] / "instructions.md") == "Offline B.\n"
        # Roll back a full generation, then restart after recovery.
        (a["source"] / "instructions.md").write_text("Undo this.\n")
        a["cli"]("sync", "--local")
        a["cli"]("rollback")
        assert content(a["source"] / "instructions.md") == "Offline B.\n"
        assert json.loads(a["cli"]("status", "--json").stdout)["paused"]
        a["cli"]("daemon", "resume")
        a["cli"]("sync")
        # Examine every committed blob, not just the working snapshot.
        objects = subprocess.check_output(["git", "--git-dir=" + str(remote), "rev-list", "--objects", "--all"], text=True)
        for line in objects.splitlines():
            oid = line.split(" ", 1)[0]
            kind = subprocess.check_output(["git", "--git-dir=" + str(remote), "cat-file", "-t", oid], text=True).strip()
            if kind == "blob":
                body = subprocess.check_output(["git", "--git-dir=" + str(remote), "cat-file", "blob", oid])
                assert b"sentinel-must-stay-local" not in body
                assert b"sentinel-private-session" not in body
        print("PASS: two real daemons; automatic import/export; offline retry; conflicts; no feedback commits; rollback/recovery; no excluded stores in Git")
    finally:
        for proc in running:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=25)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
        for log in logs:
            log.close()
