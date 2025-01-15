TOP_DIR := $(PWD)

.PHONY:clean
clean:
	rm -rf pkg/configmanager/bin

.PHONY:amddcm
amddcm:
	${MAKE} -C cmd/deviceconfigmanager TOP_DIR=$(TOP_DIR)
	${MAKE} -C docker TOP_DIR=$(TOP_DIR)

copyrights:
	GOFLAGS=-mod=mod go run tools/build/copyright/main.go && ./tools/build/check-local-files.sh
