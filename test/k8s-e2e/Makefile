.DEFAULT: all
.PHONY: all lint dcm_e2e test-ainic

# Repo root (parent of test/k8s-e2e); required for -helmchart path in dcm_e2e / test-ainic / all.
TOP_DIR ?= $(abspath $(CURDIR)/../..)

# Load registry configuration from dev.env if it exists
-include ../../dev.env

# AINIC E2E test configuration
KUBECONFIG ?= ~/.kube/config
NAMESPACE ?= kube-amd-dcm-test
REGISTRY ?= ghcr.io/rocm/device-config-manager
IMAGE_TAG ?= dev
# gocheck filter for dcm_e2e (default: all DCM tests). Example: DCM_E2E_CHECK=Test012DCMDefaultGPUConfigMapWhenUnset
DCM_E2E_CHECK ?= .*DCM.*
# SIM: skip tests that need real GPU (suite simEnable). SIMENABLED=false for AMD GPU nodes.
SIMENABLED ?= true
E2E_SIM_FLAG := $(if $(filter false,$(SIMENABLED)),-simenabled=false,)

lint:
	@go fmt ./...
	@goimports -w ./
	@go vet ./...

all:
	@TOP_DIR=$(TOP_DIR) go test -failfast -helmchart $(TOP_DIR)/helm-charts/ -test.timeout=30m -v $(TEST_ARGS);

dcm_e2e:
	@echo "Running DCM E2E tests (-check.f $(DCM_E2E_CHECK))..."
	@go test -v -count=1 $(E2E_SIM_FLAG) -check.f "$(DCM_E2E_CHECK)" \
		-kubeconfig $(KUBECONFIG) \
		-helmchart $(TOP_DIR)/helm-charts/ \
		-namespace $(NAMESPACE) \
		-registry $(REGISTRY) \
		-imagetag $(IMAGE_TAG) \
		-timeout 30m

test-ainic:
	@echo "Running AINIC E2E tests..."
	@go test -v -check.f ".*AINIC.*" \
		-kubeconfig $(KUBECONFIG) \
		-helmchart $(TOP_DIR)/helm-charts/ \
		-namespace $(NAMESPACE) \
		-registry $(REGISTRY) \
		-imagetag $(IMAGE_TAG) \
		-timeout 30m 
