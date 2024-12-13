OUT_DIR := bin
TOP_DIR := $(PWD)

export ${OUT_DIR}

.PHONY:clean
clean:
	rm -rf pkg/config_manager/bin

.PHONY:amddcm
amddcm:
	${MAKE} -C pkg/config_manager TOP_DIR=$(TOP_DIR)
	${MAKE} -C docker TOP_DIR=$(TOP_DIR)

