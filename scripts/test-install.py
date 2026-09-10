#!/usr/bin/env python3
"""Installer/release lifecycle tests, with disposable homes and local fixtures."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument("binary", type=Path)
parser.add_argument("--verify-unit", action="store_true")
args = parser.parse_args()
binary = args.binary.resolve()
repo = Path(__file__).resolve().parent.parent
version = json.loads((repo / "manifest.json").read_text())["version"]


def archive_at(folder, payload, unsafe=False):
    name = f"omai_{version}_linux_amd64.tar.gz"
    path = folder / name
    with tarfile.open(path, "w:gz") as bundle:
        for label, data in [("omai", payload), ("LICENSE", b"MIT\n")]:
            info = tarfile.TarInfo(label)
            info.size, info.mode = len(data), 0o755
            bundle.addfile(info, io.BytesIO(data))
        if unsafe:
            info = tarfile.TarInfo("../outside")
            info.type, info.linkname = tarfile.SYMTYPE, "/tmp/outside"
            bundle.addfile(info)
    (folder / "SHA256SUMS").write_text(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + name + "\n")
    return path


with tempfile.TemporaryDirectory(prefix="omai-install-test-") as tmp:
    root = Path(tmp)
    plugin = root / "readonly plugin"
    plugin.mkdir()
    shutil.copytree(repo / "scripts", plugin / "scripts")
    for name in ("manifest.json", "mise.toml"):
        shutil.copy2(repo / name, plugin / name)
    fixtures, tools_dir = root / "fixtures", root / "tools"
    fixtures.mkdir()
    tools_dir.mkdir()
    archive = archive_at(fixtures, binary.read_bytes())
    fake_tool = '''#!PYTHON
import json, os, pathlib, shutil, sys, time
root=pathlib.Path(os.environ["OMAI_INSTALL_TEST"])
args=sys.argv[1:]
tool=pathlib.Path(sys.argv[0]).name
with (root/"calls").open("a") as out: out.write(json.dumps([tool]+args)+"\\n")
if tool=="curl":
 if (root/"download-fail").exists(): sys.exit(22)
 assert args[-1].startswith("https://github.com/pablousx/omai/releases/download/v"+os.environ["OMAI_TEST_VERSION"]+"/")
 assert "--proto" in args and "--proto-redir" in args
 target=args[args.index("--output")+1]
 shutil.copyfile(root/"fixtures"/args[-1].rsplit("/",1)[1],target)
elif tool=="mise":
 assert args[0]=="exec" and args[1]=="go@"+os.environ["OMAI_TEST_GO"]
 target=args[args.index("-o")+1]
 shutil.copyfile(os.environ["OMAI_TEST_BINARY"],target)
 module=pathlib.Path(os.environ["GOMODCACHE"])/"fixture-module"
 module.mkdir(parents=True)
 (module/"go.mod").write_text("module fixture")
 if "-modcacherw" not in args: module.chmod(0o555)
elif tool=="systemctl":
 action=args[1]
 if action=="is-active": sys.exit(0 if (root/"active").exists() else 3)
 elif action=="is-enabled": sys.exit(0 if (root/"enabled").exists() else 1)
 elif action=="stop": (root/"active").unlink(missing_ok=True)
 elif action=="disable":
  if (root/"stop-fail").exists(): sys.exit(1)
  (root/"enabled").unlink(missing_ok=True)
  if "--now" in args: (root/"active").unlink(missing_ok=True)
 elif action=="start" or (action=="enable" and "--now" in args):
  if (root/"activation-fail").exists():
   (root/"activation-fail").unlink();sys.exit(1)
  (root/"active").touch()
  if action=="enable": (root/"enabled").touch()
 elif action=="enable": (root/"enabled").touch()
 elif action=="daemon-reload":
  if (root/"interrupt").exists():
   (root/"at-reload").touch()
   time.sleep(60)
 else: sys.exit(1)
'''.replace("PYTHON", sys.executable)
    for tool in ("curl", "systemctl", "mise"):
        path = tools_dir / tool
        path.write_text(fake_tool)
        path.chmod(0o700)
    import tomllib
    pin = tomllib.loads((repo / "mise.toml").read_text())["tools"]["go"]
    home = root / 'home with space % and "quote"'
    home.mkdir()
    temp = root / "tmp"
    temp.mkdir()
    (root / "runtime").mkdir(mode=0o700)
    env = {k: v for k, v in os.environ.items() if not k.startswith(("XDG_", "CODEX_", "CLAUDE_", "OPENCODE_", "GIT_")) and k != "DBUS_SESSION_BUS_ADDRESS"}
    env.update(HOME=str(home), XDG_CONFIG_HOME=str(root / "custom config"),
               XDG_DATA_HOME=str(root / "custom data"), XDG_STATE_HOME=str(root / "custom state"),
               XDG_CACHE_HOME=str(root / "cache"), XDG_RUNTIME_DIR=str(root / "runtime"), TMPDIR=str(temp),
               PATH=str(tools_dir) + ":" + env["PATH"], OMAI_INSTALL_TEST=str(root),
               OMAI_TEST_VERSION=version, OMAI_TEST_GO=pin, OMAI_TEST_BINARY=str(binary))
    dest = Path(env["XDG_DATA_HOME"]) / "omai/bin/omai"
    state = Path(env["XDG_STATE_HOME"]) / "omai"
    config = Path(env["XDG_CONFIG_HOME"]) / "omai"
    unit = Path(env["XDG_CONFIG_HOME"]) / "systemd/user/omai.service"
    cli = home / ".local/bin/omai"
    source_hashes = {str(p.relative_to(plugin)): hashlib.sha256(p.read_bytes()).hexdigest()
                     for p in plugin.rglob("*") if p.is_file()}
    for p in plugin.rglob("*"):
        p.chmod(0o555 if p.is_dir() or os.access(p, os.X_OK) else 0o444)
    plugin.chmod(0o555)

    def run(*command, ok=True):
        result = subprocess.run(command, env=env, text=True, capture_output=True, timeout=35)
        assert (result.returncode == 0) == ok, (command, result.returncode, result.stdout, result.stderr)
        return result

    def setup(*options, ok=True):
        return run(str(plugin / "scripts/setup"), "--install-only", *options, ok=ok)

    try:
        # The plugin form provisions a missing executable and starts the service
        # without stdin. A retry with the current executable works offline.
        plugin_bridge = str(plugin / "scripts/plugin-setup")
        run(plugin_bridge, "setup", "--remote", "", "--machine", "Plugin computer", "--seed", "")
        assert dest.exists() and unit.exists() and (root / "active").exists()
        assert json.loads((config / "config.json").read_text())["machine"] == "Plugin computer"
        (root / "download-fail").touch()
        run(plugin_bridge, "setup", "--remote", "", "--machine", "Plugin computer", "--seed", "")
        prior = dest.read_bytes()
        run(plugin_bridge, "update", ok=False)
        assert dest.read_bytes() == prior
        run(plugin_bridge, "update", "--source")
        assert dest.read_bytes() == prior and not list(temp.iterdir())
        (root / "download-fail").unlink()
        source_before = (config / "source/omai.json").read_bytes()
        run(str(dest), "settings", "clear", ok=False)
        assert (config / "config.json").exists(), "reset without confirmation changed settings"
        (root / "stop-fail").touch()
        run(str(dest), "settings", "clear", "--yes", ok=False)
        assert (config / "config.json").exists() and (root / "active").exists(), "failed stop cleared settings"
        (root / "stop-fail").unlink()
        run(str(dest), "settings", "clear", "--yes")
        assert not (config / "config.json").exists() and not (root / "active").exists() and not (root / "enabled").exists()
        assert (config / "source/omai.json").read_bytes() == source_before
        run(plugin_bridge, "setup", "--remote", "", "--machine", "Fresh preferences", "--seed", "")
        assert (root / "active").exists() and json.loads((config / "config.json").read_text())["machine"] == "Fresh preferences"
        run(str(dest), "daemon", "uninstall")
        for folder in (config, state, dest.parent.parent):
            shutil.rmtree(folder)
        assert not (root / "active").exists()

        # Failures before activation leave the existing executable untouched.
        dest.parent.mkdir(parents=True)
        sentinel = b"old installed binary\n"
        dest.write_bytes(sentinel)
        (root / "download-fail").touch()
        setup(ok=False)
        assert dest.read_bytes() == sentinel
        (root / "download-fail").unlink()
        (fixtures / "SHA256SUMS").write_text("0" * 64 + "  " + archive.name + "\n")
        setup(ok=False)
        assert dest.read_bytes() == sentinel
        archive_at(fixtures, binary.read_bytes(), unsafe=True)
        setup(ok=False)
        assert dest.read_bytes() == sentinel and not (root / "outside").exists()
        archive_at(fixtures, b"#!/bin/sh\necho 0.0.0\n")
        setup(ok=False)
        assert dest.read_bytes() == sentinel
        archive_at(fixtures, binary.read_bytes())
        setup()
        assert run(str(dest), "version").stdout.strip() == version
        assert not unit.exists(), "install before onboarding created a service"
        assert Path(str(dest) + ".previous").read_bytes() == sentinel
        setup("--source")
        assert not list(temp.iterdir()), "source setup left temporary module caches"
        setup("--build-only")
        assert not list(temp.iterdir()), "build-only setup left temporary module caches"

        run(str(dest), "setup", "--yes", "--no-service", "--providers", "codex")
        (config / "source/instructions.md").write_text("Keep configuration across updates.\n")
        cli.parent.mkdir(parents=True, exist_ok=True)
        cli.write_text("unrelated CLI")
        setup(ok=False)
        assert cli.read_text() == "unrelated CLI"
        cli.unlink()
        setup()
        assert unit.exists() and cli.exists()
        if args.verify_unit:
            run("systemd-analyze", "--user", "verify", str(unit))
        assert not (root / "active").exists(), "update started stopped service"
        run(str(dest), "daemon", "pause")
        (root / "active").touch()
        (root / "enabled").touch()
        prior_unit = unit.read_bytes()
        (root / "activation-fail").touch()
        setup(ok=False)
        assert (root / "active").exists() and (root / "enabled").exists()
        assert unit.read_bytes() == prior_unit and (state / "paused.json").exists()

        # Kill a real updater after it writes installation files, before activation.
        unit.write_text("# Managed by omai\nold unit to recover\n")
        (root / "interrupt").touch()
        process = subprocess.Popen([str(plugin / "scripts/setup"), "--install-only"], env=env,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            end = time.monotonic() + 15
            while not (root / "at-reload").exists() and time.monotonic() < end:
                assert process.poll() is None, "updater exited before interruption point"
                time.sleep(0.05)
            assert (root / "at-reload").exists(), "updater never reached activation"
            assert (state / "install-pending.json").exists()
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)
            (root / "interrupt").unlink()
        run(str(dest), "install", "--recover")
        assert unit.read_text() == "# Managed by omai\nold unit to recover\n"
        assert (root / "active").exists() and not (state / "install-pending.json").exists()
        setup()
        assert (state / "paused.json").exists()
        run(str(dest), "daemon", "uninstall")
        assert not unit.exists() and not cli.exists()
        assert not (root / "active").exists() and not (root / "enabled").exists()
        assert (config / "source/instructions.md").read_text() == "Keep configuration across updates.\n"
        assert dest.exists() and (state / "install-backups/latest.json").exists()
        assert source_hashes == {str(p.relative_to(plugin)): hashlib.sha256(p.read_bytes()).hexdigest()
                                 for p in plugin.rglob("*") if p.is_file()}
        print("PASS: verified downloads; failed checksums/archives/versions; source fallback; readonly plugin; custom XDG; interrupted/failed upgrades; service state; safe removal")
    finally:
        plugin.chmod(0o700)
        for p in plugin.rglob("*"):
            p.chmod(0o700 if p.is_dir() else 0o600)
