#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_TOKEN:?set GITHUB_TOKEN}"
: "${NOVITA_API_KEY:?set NOVITA_API_KEY}"
: "${GITHUB_CONFIG_URL:?set GITHUB_CONFIG_URL to the target GitHub repository or organization URL}"
if [[ "$GITHUB_CONFIG_URL" == *OWNER* || "$GITHUB_CONFIG_URL" == *REPOSITORY* || "$GITHUB_CONFIG_URL" == *REPO ]]; then
  printf 'GITHUB_CONFIG_URL still contains an example placeholder: %s\n' "$GITHUB_CONFIG_URL" >&2
  exit 1
fi
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/hosted-common.sh
source "$script_dir/hosted-common.sh"
hosted_require_novita_cli
github_config_url=$(hosted_normalize_github_config_url "$GITHUB_CONFIG_URL")
target_id=$(hosted_target_id "$github_config_url")
controller_template=${CONTROLLER_TEMPLATE:-github-actions-controller}
runner_template=${NOVITA_RUNNER_TEMPLATE:-github-actions-runner}
control_timeout=${CONTROL_TIMEOUT:-525600h}
github_secret=${GITHUB_SECRET_NAME:-novita-actions-github-$target_id}
novita_secret=${NOVITA_SECRET_NAME:-novita-actions-novita-$target_id}
controller_id=${CONTROLLER_ID:-novita-hosted-$target_id}

# These Secrets are dependencies of a resumable Control Sandbox. Keep and
# rotate them in place; never delete them until that Sandbox is killed.
if novita secret get "$github_secret" >/dev/null 2>&1; then
  novita secret update "$github_secret" --host github.com --host api.github.com \
    --host '*.actions.githubusercontent.com' --from-env GITHUB_TOKEN
else
  novita secret create "$github_secret" --host github.com --host api.github.com \
    --host '*.actions.githubusercontent.com' --from-env GITHUB_TOKEN
fi
if novita secret get "$novita_secret" >/dev/null 2>&1; then
  novita secret update "$novita_secret" --host api.us-phx-1.sandbox.novita.ai \
    --host us-phx-1.sandbox.novita.ai \
    --host '*.us-phx-1.sandbox.novita.ai' --from-env NOVITA_API_KEY
else
  novita secret create "$novita_secret" --host api.us-phx-1.sandbox.novita.ai \
    --host us-phx-1.sandbox.novita.ai \
    --host '*.us-phx-1.sandbox.novita.ai' --from-env NOVITA_API_KEY
fi

resolved=$(hosted_resolve_controller "$target_id" "$controller_id")
existing_state=
existing=
if [[ -n "$resolved" ]]; then
  read -r existing_state existing <<<"$resolved"
fi
if [[ "$existing_state" == paused ]]; then
  novita sandbox resume "$existing" --timeout "$control_timeout" >/dev/null
fi
# 525600 hours is 60 * 365 days. Keep both Secrets above for this entire
# resumable Sandbox lifetime; kill the Sandbox before deleting either Secret.
if [[ -z "$existing" ]]; then
  created=$(novita sandbox create "$controller_template" --detach --long-running \
    --timeout "$control_timeout" --on-timeout pause --auto-resume --idle-timeout 0 \
    --metadata managed_by=novita-actions-runner-controller \
    --metadata role=control-plane --metadata target_id="$target_id" \
    --metadata controller_id="$controller_id" \
    --secret-env GITHUB_TOKEN="$github_secret" \
    --secret-env NOVITA_API_KEY="$novita_secret" \
    --env GITHUB_CONFIG_URL="$github_config_url" \
    --env NOVITA_RUNNER_TEMPLATE="$runner_template" \
    --env SCALE_SET_NAME="${SCALE_SET_NAME:-novita-scale-set}" \
    --env RUNNER_LABELS="${RUNNER_LABELS:-novita-scale-set}" \
    --env CONTROLLER_ID="$controller_id" \
    --env MAX_RUNNERS="${MAX_RUNNERS:-10}" --output json)
  existing=$(printf '%s' "$created" | python3 -c 'import json,sys; print(json.load(sys.stdin)["sandboxId"])')
fi

# Template start processes are restored from the build snapshot and therefore
# do not inherit creation-time env. A post-create exec does; detach it from the
# exec session so it survives after this script returns and across pause/resume.
if ! novita sandbox exec "$existing" -- pgrep -f \
  '^/usr/local/bin/novita-actions-controller$' >/dev/null 2>&1; then
  novita sandbox exec "$existing" --background -- \
    /usr/local/bin/novita-actions-controller >/dev/null
fi
for attempt in {1..30}; do
  if novita sandbox exec "$existing" -- python -c \
    'import urllib.request; assert urllib.request.urlopen("http://127.0.0.1:8080/healthz", timeout=2).status == 200' \
    >/dev/null 2>&1; then
    printf '{"targetId":"%s","controllerId":"%s","sandboxId":"%s","healthy":true}\n' \
      "$target_id" "$controller_id" "$existing"
    exit 0
  fi
  sleep 2
done
printf 'Hosted controller did not become healthy; inspect /home/user/novita-actions-controller.log in %s\n' "$existing" >&2
exit 1
