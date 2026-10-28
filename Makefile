VERSION ?= v0.1.1
CONTROLLER_IMAGE ?= novitalabs/github-actions-controller
RUNNER_IMAGE ?= novitalabs/github-actions-runner
CONTROLLER_TEMPLATE ?= github-actions-controller
RUNNER_TEMPLATE ?= github-actions-runner

.PHONY: test build images runner-template controller-template publish-templates
test:
	bash scripts/test-hosted.sh
	go test ./...
	PYTHONPATH=provisioner/novita python -m pytest -q provisioner/novita/test_adapter.py
build:
	CGO_ENABLED=0 go build -trimpath -o controller ./cmd/novita-actions-controller
images:
	scripts/release-images.sh $(VERSION)
runner-template:
	novita template create $(RUNNER_TEMPLATE) --path templates/runner --dockerfile novita.Dockerfile --cpu-count 2 --memory-mb 4096 --cmd /home/user/actions-runner/entrypoint.sh --ready-cmd 'test -x /home/user/actions-runner/entrypoint.sh'
controller-template:
	novita template create $(CONTROLLER_TEMPLATE) --path . --dockerfile templates/controller/novita.Dockerfile --cpu-count 1 --memory-mb 1024 --cmd /usr/local/bin/novita-actions-controller-entrypoint --ready-cmd 'test -x /usr/local/bin/novita-actions-controller-entrypoint'
publish-templates:
	novita template publish $(RUNNER_TEMPLATE) -y
	novita template publish $(CONTROLLER_TEMPLATE) -y
