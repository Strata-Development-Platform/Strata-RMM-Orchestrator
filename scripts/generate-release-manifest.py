#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
from datetime import datetime, timezone
from pathlib import Path

SHA256_RE = re.compile(r"^[0-9a-fA-F]{64}$")
OCI_DIGEST_RE = re.compile(r"^sha256:[0-9a-fA-F]{64}$")
SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")
SEMVER_RE = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$")


def parse_args():
    parser = argparse.ArgumentParser(description="Generate the Strata RMM release provenance manifest")
    parser.add_argument("--dist", default="dist")
    parser.add_argument("--version", required=True)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--minimum-upgrade-version", required=True)
    parser.add_argument("--schema-compatibility", required=True)
    parser.add_argument("--channel", choices=("stable", "prerelease"), required=True)
    parser.add_argument("--oci-image", required=True)
    parser.add_argument("--oci-digest", required=True)
    parser.add_argument("--output", default="dist/release-manifest.json")
    return parser.parse_args()


def load_checksums(path: Path):
    values = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        parts = raw.split()
        if len(parts) != 2:
            raise SystemExit(f"invalid checksum line: {raw!r}")
        digest, name = parts[0].lower(), parts[1].lstrip("*")
        if not SHA256_RE.fullmatch(digest):
            raise SystemExit(f"invalid sha256 for {name}")
        if name in values:
            raise SystemExit(f"duplicate checksum entry for {name}")
        values[name] = digest
    if not values:
        raise SystemExit("checksums.txt contained no artifacts")
    return values


def classify(name: str):
    lower = name.lower()
    if "orchestrator" in lower:
        kind = "orchestrator"
    elif "agent" in lower:
        kind = "agent"
    elif "probe" in lower:
        kind = "probe"
    else:
        kind = "release"
    os_name = next((v for v in ("linux", "windows", "darwin") if f"-{v}-" in lower or f"_{v}_" in lower), "")
    arch = next((v for v in ("amd64", "arm64") if v in lower), "")
    return kind, os_name, arch


def main():
    args = parse_args()
    version = args.version.removeprefix("v")
    minimum = args.minimum_upgrade_version.removeprefix("v")
    if not SEMVER_RE.fullmatch(version):
        raise SystemExit("version must be semantic version syntax")
    if not SEMVER_RE.fullmatch(minimum):
        raise SystemExit("minimum upgrade version must be semantic version syntax")
    if not SHA_RE.fullmatch(args.source_sha):
        raise SystemExit("source SHA must be a full 40-character git SHA")
    if not args.oci_image.strip() or "://" in args.oci_image or "@" in args.oci_image:
        raise SystemExit("OCI image must be an untagged registry/repository reference")
    if not OCI_DIGEST_RE.fullmatch(args.oci_digest):
        raise SystemExit("OCI digest must be sha256:<64 hex characters>")

    dist = Path(args.dist)
    checksums = load_checksums(dist / "checksums.txt")
    artifacts = []
    for name in sorted(checksums):
        artifact = dist / name
        if not artifact.is_file():
            raise SystemExit(f"checksum entry does not resolve to a release artifact: {name}")
        actual = hashlib.sha256(artifact.read_bytes()).hexdigest()
        if actual != checksums[name]:
            raise SystemExit(f"checksum mismatch while building manifest: {name}")
        bundle = dist / f"{name}.sigstore.json"
        if not bundle.is_file():
            raise SystemExit(f"release artifact lacks Sigstore bundle: {name}")
        kind, os_name, arch = classify(name)
        artifacts.append({
            "name": name,
            "kind": kind,
            "os": os_name,
            "arch": arch,
            "size": artifact.stat().st_size,
            "sha256": checksums[name],
            "sigstore_bundle": bundle.name,
        })

    manifest = {
        "schema_version": 1,
        "version": version,
        "source_sha": args.source_sha.lower(),
        "build_timestamp": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
        "schema_compatibility": args.schema_compatibility,
        "minimum_upgrade_version": minimum,
        "channel": args.channel,
        "artifacts": artifacts,
        "oci_images": [{
            "reference": args.oci_image.lower(),
            "digest": args.oci_digest.lower(),
            "platforms": ["linux/amd64", "linux/arm64"],
            "signature": "sigstore-keyless",
        }],
    }
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(output)


if __name__ == "__main__":
    main()
