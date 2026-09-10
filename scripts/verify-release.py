#!/usr/bin/env python3
"""Verify a release checksum and extract only its regular executable."""
import hashlib
from pathlib import Path
import sys
import tarfile

stage, name = Path(sys.argv[1]), sys.argv[2]
archive = stage / name
if archive.stat().st_size > 64 * 1024 * 1024:
    raise SystemExit("Release archive exceeds 64 MiB")
entries = [line.split() for line in (stage / "SHA256SUMS").read_text().splitlines()]
checks = [parts[0] for parts in entries if len(parts) == 2 and parts[1] == name]
if len(checks) != 1 or hashlib.sha256(archive.read_bytes()).hexdigest() != checks[0]:
    raise SystemExit("Release checksum verification failed; installation unchanged")
with tarfile.open(archive, "r:gz") as bundle:
    members = bundle.getmembers()
    if sorted(m.name for m in members) != ["LICENSE", "omai"]:
        raise SystemExit("Unexpected release archive contents")
    for member in members:
        if not member.isfile() or member.size > 64 * 1024 * 1024:
            raise SystemExit("Unsafe release archive member")
    with bundle.extractfile("omai") as source:
        with (stage / "omai").open("xb") as target:
            target.write(source.read())
