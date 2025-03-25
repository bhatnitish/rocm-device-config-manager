TOP_DIR := $(PWD)
HELM_CHARTS_DIR := $(TOP_DIR)/helm-charts
PKG_LIB_PATH := ${TOP_DIR}/debian/usr/local/
ASSETS_PATH :=${TOP_DIR}/assets
# 22.04 - jammy
# 24.04 - noble
UBUNTU_VERSION ?= jammy
UBUNTU_VERSION_NUMBER = 22.04
ifeq (${UBUNTU_VERSION}, noble)
UBUNTU_VERSION_NUMBER = 24.04
endif

ifeq ($(RELEASE),)
DEBIAN_VERSION := "1.0.0"
else
DEBIAN_VERSION := $(shell echo "$(RELEASE)" | sed 's/^.//')
endif

DEBIAN_CONTROL = ${TOP_DIR}/debian/DEBIAN/control
BUILD_VER_ENV = ${DEBIAN_VERSION}~$(UBUNTU_VERSION_NUMBER)

AMD_SMI_LIBS := ${ASSETS_PATH}/amd_smi_lib/x86_64/${UBUNTU_VERSION}/lib
PKG_PATH := ${TOP_DIR}/debian/usr/local/bin

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

pkg-clean:
	rm -rf ${TOP_DIR}/bin/*.deb

.PHONY: pkg
pkg: pkg-clean
	${MAKE} dcm
	@echo "Building debian for $(BUILD_VER_ENV)"
	#copy precompiled libs
	mkdir -p ${PKG_LIB_PATH}
	cp -rvf ${AMD_SMI_LIBS}/ ${PKG_LIB_PATH}
	mkdir -p ${PKG_PATH}
	cp -vf $(TOP_DIR)/bin/device-config-manager ${PKG_PATH}/
	#strip the dcm gobin to reduce the debian package size
	strip ${PKG_PATH}/device-config-manager
	cd ${TOP_DIR}
	sed -i "s/BUILD_VER_ENV/$(BUILD_VER_ENV)/g" $(DEBIAN_CONTROL)
	dpkg-deb -Zxz --build debian ${TOP_DIR}/bin
	#remove copied files
	rm -rf ${PKG_LIB_PATH}
	# revert the dynamic version set file
	git checkout $(DEBIAN_CONTROL)
	# rename for internal build
	mv -vf ${TOP_DIR}/bin/amdgpu-configmanager_*~${UBUNTU_VERSION_NUMBER}_amd64.deb ${TOP_DIR}/bin/amdgpu-configmanager_${UBUNTU_VERSION_NUMBER}_amd64.deb

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