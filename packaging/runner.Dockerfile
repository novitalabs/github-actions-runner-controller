FROM debian:bookworm-slim
ARG RUNNER_VERSION=2.337.0
ARG TARGETARCH
ARG VERSION=dev
LABEL org.opencontainers.image.source="https://github.com/garysng/novita-actions-runner-controller" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.title="Novita GitHub Actions Runner" \
      io.novita.github-actions.runner-version="${RUNNER_VERSION}"
RUN apt-get update && apt-get install -y --no-install-recommends bash ca-certificates curl git jq libicu72 libssl3 unzip && rm -rf /var/lib/apt/lists/*
RUN id user >/dev/null 2>&1 || useradd --create-home --uid 1000 user
RUN mkdir -p /home/user/actions-runner && chown -R user:user /home/user
USER user
WORKDIR /home/user/actions-runner
ARG RUNNER_DOWNLOAD_BASE=https://github.com
ARG RUNNER_SHA256_AMD64=70920811a4f8ad4328818682bca5c6469c1c942fab52448868071d0063816613
ARG RUNNER_SHA256_ARM64=9b1dc70626422526e3c94767cf024896beb15da5342a3f4819bf2feac13e0393
RUN case "${TARGETARCH}" in \
      amd64) runner_arch=x64; runner_sha256="${RUNNER_SHA256_AMD64}" ;; \
      arm64) runner_arch=arm64; runner_sha256="${RUNNER_SHA256_ARM64}" ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && curl --fail --show-error --location \
      --retry 5 --retry-all-errors --connect-timeout 15 --max-time 900 \
      --output /tmp/actions-runner.tar.gz \
      "${RUNNER_DOWNLOAD_BASE}/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${runner_arch}-${RUNNER_VERSION}.tar.gz" \
    && echo "${runner_sha256}  /tmp/actions-runner.tar.gz" | sha256sum --check --strict \
    && tar xzf /tmp/actions-runner.tar.gz \
    && rm /tmp/actions-runner.tar.gz
COPY --chown=user:user --chmod=755 templates/runner/entrypoint.sh /home/user/actions-runner/entrypoint.sh
CMD ["/home/user/actions-runner/entrypoint.sh"]
