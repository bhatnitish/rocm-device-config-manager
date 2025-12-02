.DEFAULT: all
.PHONY: all lint test-ainic

# Load registry configuration from dev.env if it exists
# This allows separating internal registry details from public Makefile
-include ../../dev.env

TEST_ARGS = ""
ifdef OPENSHIFT
	TEST_ARGS += "-openshift"
endif

ifdef SIM_ENABLE
	TEST_ARGS += "-simEnable"
endif

ifdef CI_ENV
	TEST_ARGS += "-ciEnv"
endif

# AINIC E2E test configuration
KUBECONFIG ?= ~/.kube/config
NAMESPACE ?= kube-amd-dcm-test
# Default to public registry, can be overridden by dev.env or environment
REGISTRY ?= ghcr.io/rocm/device-config-manager
IMAGE_TAG ?= dev

lint:
	@go fmt ./...
	@goimports -w ./
	@go vet ./...

all:
	@TOP_DIR=$(TOP_DIR) go test -failfast -helmchart $(TOP_DIR)/helm-charts/ -test.timeout=30m -v $(TEST_ARGS);

test-ainic:
	@echo "Running AINIC E2E tests..."
	@go test -v -check.f ".*AINIC.*" \
		-kubeconfig $(KUBECONFIG) \
		-helmchart $(TOP_DIR)/helm-charts/ \
		-namespace $(NAMESPACE) \
		-registry $(REGISTRY) \
		-imagetag $(IMAGE_TAG) \
		-timeout 30m 
