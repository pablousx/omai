#!/usr/bin/env python3
"""Keep release metadata and distributable contents consistent."""
import argparse
import json
from pathlib import Path
import re
import subprocess
import tomllib

parser = argparse.ArgumentParser()
parser.add_argument("--binary", type=Path)
parser.add_argument("--tag")
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
manifest = json.loads((root / "manifest.json").read_text())
version = manifest["version"]
assert re.fullmatch(r"\d+\.\d+\.\d+", version), "release version must be stable SemVer"
assert manifest["schemaVersion"] == 1 and manifest["id"] == "pablousx.omai"
assert f'const Version = "{version}"' in (root / "internal/omai/model.go").read_text(), "Go version mismatch"
assert f'"version":"{version}"' in (root / "scripts/omai").read_text(), "launcher version mismatch"
assert tomllib.loads((root / "mise.toml").read_text())["tools"]["go"], "missing Go pin"
if args.tag:
    assert args.tag == "v" + version, "tag does not match manifest"
if args.binary:
    actual = subprocess.check_output([str(args.binary.resolve()), "version"], text=True).strip()
    assert actual == version, "binary does not match manifest"
for entry in manifest["entryPoints"].values():
    assert not Path(entry).is_absolute() and ".." not in Path(entry).parts
    assert (root / entry).is_file()
for path in root.rglob("*"):
    if any(part in (".git", "build", ".agents", ".codex") for part in path.relative_to(root).parts):
        continue
    assert not path.is_symlink(), f"plugin contains symlink: {path}"
print(f"PASS: omai {version} release metadata")
