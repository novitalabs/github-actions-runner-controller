FROM novitalabs/github-actions-runner:v0.1.1
# Preinstall a C toolchain so native build scripts (Rust `cc`, Python wheels,
# Node native addons) can link. The base image is debian:bookworm-slim and
# contains no linker; without this, `cargo build` fails with "linker `cc` not found".
USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential pkg-config libssl-dev zstd \
    && rm -rf /var/lib/apt/lists/*
USER user
CMD ["/home/user/actions-runner/entrypoint.sh"]
