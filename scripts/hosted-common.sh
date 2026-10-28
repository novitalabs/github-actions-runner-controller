#!/usr/bin/env bash

# Shared target identity and lookup helpers for Hosted deployments.

hosted_require_novita_cli() {
  command -v novita >/dev/null 2>&1 || {
    printf 'novita CLI is required; install it with npm install --global novita-sandbox-cli@latest\n' >&2
    return 1
  }
  local version
  version=$(novita --version 2>&1 | head -n 1)
  python3 - "$version" <<'PY'
import re
import sys

match = re.search(r"(\d+)\.(\d+)\.(\d+)", sys.argv[1])
if match is None or tuple(map(int, match.groups())) < (2, 1, 0):
    raise SystemExit(
        f"novita CLI >= 2.1.0 is required; found {sys.argv[1]!r}. "
        "Install the latest version with npm install --global "
        "novita-sandbox-cli@latest"
    )
PY
}

hosted_normalize_github_config_url() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import urlsplit

raw = sys.argv[1].strip()
parsed = urlsplit(raw)
parts = [part for part in parsed.path.split("/") if part]
if (
    parsed.scheme.lower() != "https"
    or parsed.hostname is None
    or parsed.hostname.lower() != "github.com"
    or parsed.username is not None
    or parsed.password is not None
    or parsed.port is not None
    or parsed.query
    or parsed.fragment
    or len(parts) not in (1, 2)
):
    raise SystemExit(
        "GITHUB_CONFIG_URL must be an https://github.com/OWNER or "
        "https://github.com/OWNER/REPOSITORY URL"
    )
if len(parts) == 2 and parts[1].lower().endswith(".git"):
    parts[1] = parts[1][:-4]
if any(not part for part in parts):
    raise SystemExit("GITHUB_CONFIG_URL contains an empty owner or repository")
print("https://github.com/" + "/".join(part.lower() for part in parts))
PY
}

hosted_target_id() {
  python3 - "$1" <<'PY'
import hashlib
import re
import sys
from urllib.parse import urlsplit

url = sys.argv[1]
path = urlsplit(url).path.strip("/")
slug = re.sub(r"[^a-z0-9]+", "-", path.lower()).strip("-")[:24].rstrip("-")
digest = hashlib.sha256(url.encode()).hexdigest()[:10]
print(f"{slug}-{digest}")
PY
}

hosted_find_controller() {
  local state=$1
  local target_id=$2
  local controller_id=$3
  novita sandbox list --all --state "$state" \
    --metadata managed_by=novita-actions-runner-controller \
    --metadata role=control-plane --metadata target_id="$target_id" \
    --metadata controller_id="$controller_id" --output json | \
    python3 -c '
import json
import sys
state, target_id = sys.argv[1:]
data = json.load(sys.stdin)
items = data if isinstance(data, list) else data.get("sandboxes", [])
ids = [item.get("sandboxId") or item.get("sandbox_id") for item in items]
ids = [sandbox_id for sandbox_id in ids if sandbox_id]
if len(ids) > 1:
    print(f"multiple {state} Control Sandboxes match target {target_id}: " + ", ".join(ids), file=sys.stderr)
    raise SystemExit(2)
print(ids[0] if ids else "")
' "$state" "$target_id"
}

hosted_resolve_controller() {
  local target_id=$1
  local controller_id=$2
  local running paused
  running=$(hosted_find_controller running "$target_id" "$controller_id")
  paused=$(hosted_find_controller paused "$target_id" "$controller_id")
  if [[ -n "$running" && -n "$paused" ]]; then
    printf 'both running and paused Control Sandboxes match target %s: %s, %s\n' \
      "$target_id" "$running" "$paused" >&2
    return 1
  fi
  if [[ -n "$running" ]]; then
    printf 'running %s\n' "$running"
  elif [[ -n "$paused" ]]; then
    printf 'paused %s\n' "$paused"
  fi
}

hosted_sandbox_state() {
  novita sandbox info "$1" --output json | python3 -c '
import json
import sys
data = json.load(sys.stdin)
state = str(data.get("state", "")).lower()
if state not in {"running", "paused"}:
    print(f"unsupported or missing Sandbox state: {state!r}", file=sys.stderr)
    raise SystemExit(1)
print(state)
'
}
