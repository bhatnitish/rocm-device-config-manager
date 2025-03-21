TOP_DIR := $(PWD)
HELM_CHARTS_DIR := $(TOP_DIR)/helm-charts

.PHONY:clean
clean:
	rm -rf pkg/configmanager/bin

.PHONY: dcm
dcm:
	${MAKE} -C cmd/deviceconfigmanager TOP_DIR=$(TOP_DIR)

.PHONY: dcm-docker
dcm-docker:
	${MAKE} -C docker TOP_DIR=$(TOP_DIR)

.PHONY: docker-publish
docker-publish:
	${MAKE} -C docker docker-publish TOP_DIR=$(TOP_DIR)

.PHONY:all
all:
	${MAKE} -C cmd/deviceconfigmanager TOP_DIR=$(TOP_DIR)
	${MAKE} -C docker TOP_DIR=$(TOP_DIR)

copyrights:
	GOFLAGS=-mod=mod go run tools/build/copyright/main.go && ./tools/build/check-local-files.sh

.PHONY: helm-lint
helm-lint:
	cd $(HELM_CHARTS_DIR); helm lint

.PHONY: helm-build
helm-build: helm-lint
	helm package helm-charts/ --destination ./helm-charts

.PHONY: helm-install
helm-install: helm-build
	cd $(HELM_CHARTS_DIR); helm install amd-gpu-operator ./device-config-manager-charts-v1.0.0.tgz -n kube-amd-gpu --create-namespace -f values.yaml

.PHONY: helm-uninstall
helm-uninstall:
	helm uninstall amd-gpu-operator -n kube-amd-gpu

.PHONY: helm-list
helm-list:
	helm list --all-namespaces

GOLANGCI_LINT = $(shell pwd)/bin/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golangci-lint locally if necessary.
	$(call go-get-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.53.1)

# go-get-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
}
endef

GOFILES_NO_VENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/*")
.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	@if [ `gofmt -l $(GOFILES_NO_VENDOR) | wc -l` -ne 0 ]; then \
		echo There are some malformed files, please make sure to run \'make fmt\'; \
		gofmt -l $(GOFILES_NO_VENDOR); \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run -v --timeout 5m0s

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY:loadgpu
loadgpu:
	sudo modprobe amdgpu

.PHONY:mod
mod:
	@echo "setting up go mod packages"
	@go mod tidy
	@go mod vendor

.PHONY:checks
checks: vet

.PHONY: e2e
e2e:
	${MAKE} -C test/k8s-e2e all TOP_DIR=$(TOP_DIR)