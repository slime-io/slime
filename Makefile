SHELL=/bin/bash

IN_CONTAINER ?= 0
IMG ?= docker.io/slimeio/build-tools

root_dir = $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
GO_HEADER_FILE := $(root_dir)/boilerplate.go.txt

MODULES_ROOT ?= ./staging/src/slime.io/slime/modules
MODULES ?= lazyload limiter plugin

.PHONY: modules-api-gen
ifneq ($(MODULES_ROOT),)
ifneq ($(MODULES),)
API_PROTOS := $(foreach module,$(MODULES),$(shell find $(MODULES_ROOT)/$(module) -name "*.proto" ))
else
API_PROTOS := $(shell find $(MODULES_ROOT) -name "*.proto")
endif
.PHONY: $(API_PROTOS)
$(API_PROTOS):
	$(eval input_dir := $(shell dirname $@))
	@echo "generate proto api for $(notdir $@)"
	@protoc -I=$(input_dir) \
		-I=$(shell go env GOPATH)/src \
		--gogo_out=$(input_dir) \
		--gogo_opt=paths=source_relative \
		--gogo_opt=Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types \
		--gogo_opt=Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
		--gogo_opt=Mgoogle/protobuf/struct.proto=github.com/gogo/protobuf/types \
		--deepcopy_out=$(input_dir) \
		--deepcopy_opt=paths=source_relative \
		$@
modules-api-gen: $(API_PROTOS)
else
modules-api-gen:
endif

.PHONY: modules-k8s-gen
ifneq ($(MODULES_ROOT),)
ifneq ($(MODULES),)
MODULE_ROOTS := $(foreach module,$(MODULES),$(MODULES_ROOT)/$(module))
else
MODULE_ROOTS := $(shell find $(MODULES_ROOT) -name "go.mod" -exec dirname {} \;)
endif
.PHONY: $(MODULE_ROOTS)
$(MODULE_ROOTS):
	@echo "generate k8s object for module $(notdir $@)"
	@pushd $@ 1>/dev/null 2>&1; \
	controller-gen object:headerFile="$(GO_HEADER_FILE)" paths="./api/..."; \
	popd 1>/dev/null 2>&1
modules-k8s-gen: $(MODULE_ROOTS)
else
modules-k8s-gen:
endif

# Generate module code
.PHONY: generate-module
ifeq ($(IN_CONTAINER),1)
generate-module: modules-api-gen modules-k8s-gen
else
generate-module:
	@docker run --rm \
		--env IN_CONTAINER=1 \
		--env MODULES_ROOT=$(MODULES_ROOT) \
		--env MODULES=$(MODULES) \
		-v $(root_dir):/workspaces/slime \
		--workdir /workspaces/slime \
		--user $(shell id -u):$(shell id -g) \
		$(IMG)  \
		make $@
endif


.PHONY: shell
ifeq ($(IN_CONTAINER),1)
shell:
	@echo "already in a container"
else
shell:
	@docker run --rm -ti \
		--env IN_CONTAINER=1 \
		-v $(root_dir):/workspaces/slime \
		--workdir /workspaces/slime \
		--user $(shell id -u):$(shell id -g) \
		$(IMG)  \
		bash
endif
