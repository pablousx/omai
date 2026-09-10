#!/usr/bin/env python3
"""Build a deterministic Linux x86-64 release using the pinned Go compiler."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tomllib

parser = argparse.ArgumentParser()
parser.add_argument("--tag")
parser.add_argument("--output", type=Path, default=Path("build/release"))
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
version = json.loads((root / "manifest.json").read_text())["version"]
pin = tomllib.loads((root / "mise.toml").read_text())["tools"]["go"]
actual = subprocess.check_output(["go", "env", "GOVERSION"], cwd=root, text=True).strip()
if actual != "go" + pin:
    raise SystemExit(f"Release builds require Go {pin}; found {actual}. Run through mise.")
out = args.output.resolve()
out.mkdir(parents=True, exist_ok=True)
binary = out / "omai"
env = dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64", GOTOOLCHAIN="local", GOAMD64="v1", GOEXPERIMENT="", GOFLAGS="", GOWORK="off")
subprocess.run(["go", "build", "-buildvcs=false", "-mod=readonly", "-trimpath", "-ldflags=-s -w", "-o", str(binary), "./cmd/omai"], cwd=root, env=env, check=True)
check = ["python3", str(root / "scripts/check-release.py"), "--binary", str(binary)]
if args.tag:
    check += ["--tag", args.tag]
subprocess.run(check, check=True)
archive = out / f"omai_{version}_linux_amd64.tar.gz"
with archive.open("wb") as file:
    with gzip.GzipFile(filename="", mode="wb", fileobj=file, mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as bundle:
            for name, path, mode in [("LICENSE", root / "LICENSE", 0o644), ("omai", binary, 0o755)]:
                data = path.read_bytes()
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = len(data), mode, 0
                bundle.addfile(info, io.BytesIO(data))
(out / "SHA256SUMS").write_text(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")
print(f"Release assets: {archive} and {out / 'SHA256SUMS'}")
