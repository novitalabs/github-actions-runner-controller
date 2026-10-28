#!/usr/bin/env bash
set -euo pipefail
cd /home/user/actions-runner
jit_file=.jitconfig
until [[ -s "$jit_file" ]]; do sleep 0.2; done
export ACTIONS_RUNNER_INPUT_JITCONFIG
ACTIONS_RUNNER_INPUT_JITCONFIG="$(<"$jit_file")"
rm -f "$jit_file"
exec ./run.sh
