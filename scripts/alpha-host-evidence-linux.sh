#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --candidate-sha SHA --phase before|after [--output-dir DIR] [--package NAME]" >&2
  exit 2
}

candidate_sha=""
phase=""
output_dir="alpha-host-evidence"
package_name=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --candidate-sha) candidate_sha="${2:-}"; shift 2 ;;
    --phase) phase="${2:-}"; shift 2 ;;
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    --package) package_name="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "$candidate_sha" && ( "$phase" == "before" || "$phase" == "after" ) ]] || usage

host="$(hostname -s 2>/dev/null || hostname)"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dir="${output_dir}/${candidate_sha}/${host}/${phase}-${stamp}"
mkdir -p "$dir"

capture() {
  local name="$1"; shift
  { "$@"; } >"$dir/$name.txt" 2>&1 || true
}

cat >"$dir/manifest.txt" <<EOF
candidate_sha=$candidate_sha
phase=$phase
captured_at_utc=$stamp
hostname=$host
package=$package_name
EOF

capture uname uname -a
capture os-release cat /etc/os-release
capture identity id
capture uptime uptime
capture mounts mount
capture disk df -h
capture agent-service systemctl status strata-rmm-agent --no-pager
capture agent-journal journalctl -u strata-rmm-agent --since "-30 min" --no-pager

pm=""
for candidate in apt dnf yum zypper; do
  if command -v "$candidate" >/dev/null 2>&1; then pm="$candidate"; break; fi
done
echo "$pm" >"$dir/package-manager.txt"
case "$pm" in
  apt)
    capture patch-scan apt list --upgradable
    capture package-state dpkg-query -W -f='${Package}\t${Version}\n'
    ;;
  dnf)
    capture patch-scan dnf check-update
    capture package-state rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}\n'
    ;;
  yum)
    capture patch-scan yum check-update
    capture package-state rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}\n'
    ;;
  zypper)
    capture patch-scan zypper list-patches
    capture package-state rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}\n'
    ;;
  *) echo "No supported package manager detected" >"$dir/patch-scan.txt" ;;
esac

if [[ -n "$package_name" ]]; then
  case "$pm" in
    apt) capture selected-package dpkg-query -W "$package_name" ;;
    dnf|yum|zypper) capture selected-package rpm -q "$package_name" ;;
  esac
fi

if [[ -e /var/run/reboot-required ]]; then
  cp /var/run/reboot-required "$dir/reboot-required.txt" || true
  [[ -e /var/run/reboot-required.pkgs ]] && cp /var/run/reboot-required.pkgs "$dir/reboot-required-packages.txt" || true
else
  echo "false" >"$dir/reboot-required.txt"
fi

printf '%s\n' "$dir"
