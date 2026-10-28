#!/usr/bin/env bash
set -euo pipefail

: "${1:?usage: scripts/release-images.sh vMAJOR.MINOR.PATCH}"
version=$1
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'version must match vMAJOR.MINOR.PATCH: %s\n' "$version" >&2
  exit 1
fi
minor=${version%.*}
platforms=${PLATFORMS:-linux/amd64}

release_image() {
  local image=$1 dockerfile=$2
  local -a build_args=(--build-arg VERSION="$version")
  if [[ "$dockerfile" == packaging/Dockerfile ]]; then
    build_args+=(--build-arg "GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}")
  elif [[ -n "${RUNNER_DOWNLOAD_BASE:-}" ]]; then
    build_args+=(--build-arg "RUNNER_DOWNLOAD_BASE=$RUNNER_DOWNLOAD_BASE")
  fi
  local inspect_output
  if inspect_output=$(docker buildx imagetools inspect "$image:$version" 2>&1); then
    printf 'refusing to overwrite immutable image tag: %s:%s\n' "$image" "$version" >&2
    return 1
  fi
  if [[ ! "$inspect_output" =~ (manifest[[:space:]]unknown|not[[:space:]]found|no[[:space:]]such[[:space:]]manifest) ]]; then
    printf 'cannot verify immutable tag %s:%s; refusing to publish:\n%s\n' \
      "$image" "$version" "$inspect_output" >&2
    return 1
  fi
  docker buildx build \
    --platform "$platforms" \
    --file "$dockerfile" \
    "${build_args[@]}" \
    --tag "$image:$version" \
    --tag "$image:$minor" \
    --tag "$image:latest" \
    --push .
}

release_image novitalabs/github-actions-controller packaging/Dockerfile
release_image novitalabs/github-actions-runner packaging/runner.Dockerfile
