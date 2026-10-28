# Design notes

## Controller and provisioner boundary

The official `actions/scaleset` client handles the GitHub Scale Set protocol.
This project translates desired runner capacity and JIT configuration into
Novita Sandbox lifecycle operations. The controller owns reconciliation; the
Novita provisioner owns Sandbox creation, JIT delivery, and cleanup.

## Lifecycle ownership

Runner Sandboxes are ephemeral and default to kill on their hard timeout. No
brokered Secret is created for a runner: JIT is injected after Sandbox creation
and removed before the runner starts.

The Hosted Control Sandbox is resumable. Its GitHub and Novita brokered Secrets
are therefore durable dependencies, not bootstrap temporaries. Deleting either
while the Control Sandbox exists can make a later resume fail before the
controller process starts.
Novita applies a short default timeout on resume unless a timeout is supplied,
so operational resume must use `--timeout 525600h` (or another explicit policy).

## Packaging boundary

Local and Hosted modes use the same released controller OCI image. Maintainers
publish immutable `novitalabs/github-actions-controller:vMAJOR.MINOR.PATCH` and
`novitalabs/github-actions-runner:vMAJOR.MINOR.PATCH` images, plus rolling minor
and `latest` aliases with one release command. Public Novita Templates pin
immutable image versions and are exposed to users as
`github-actions-controller` and `github-actions-runner`. Hosted users consume
those Templates directly and do not build either image or Template.

`packaging/Dockerfile` remains a multi-stage build, so the Go toolchain is not
present in the controller runtime. The small Template Dockerfiles only add or
select the Novita-specific startup command. Novita Templates never reference a
rolling OCI tag, preventing a Template rebuild from silently changing its
runtime.

## Failure boundaries

- Scale Set message delivery is at least once. Acquiring an acquired job is a
  no-op; provisioning is idempotent by controller ID, Scale Set ID, and runner
  name.
- If JIT file injection fails after Sandbox creation, the adapter kills that
  Sandbox before returning the error.
- A missing completion event is bounded by the runner Sandbox hard timeout.
- Controller restart recovery is conservative: it preserves running Sandboxes
  and collects only aged paused Sandboxes.
