TOP_DIR := $(PWD)
HELM_CHARTS_DIR := $(TOP_DIR)/helm-charts

.PHONY:clean
clean:
	rm -rf pkg/configmanager/bin

.PHONY:amddcm
amddcm:
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