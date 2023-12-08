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
MODULES_API_PROTOS := $(foreach module,$(MODULES),$(shell find $(MODULES_ROOT)/$(module) -name "*.proto"))
else
MODULES_API_PROTOS := $(shell find $(MODULES_ROOT) -name "*.proto")
endif
.PHONY: $(MODULES_API_PROTOS)
$(MODULES_API_PROTOS):
	$(eval input_dir := $(shell dirname $@))
	@echo "generate module proto api from $@"
	@protoc -I=$(input_dir) \
		-I=$(shell go env GOPATH)/src \
		--go_out=$(input_dir) \
		--go_opt=paths=source_relative \
		--golang-deepcopy_out=$(input_dir) \
		--golang-deepcopy_opt=paths=source_relative \
		--golang-jsonshim_out=$(input_dir) \
		--golang-jsonshim_opt=paths=source_relative \
		$@
modules-api-gen: $(MODULES_API_PROTOS)
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
	controller-gen crd paths="./api/..." output:crd:dir="./charts/crds" 1>/dev/null 2>&1; \
	popd 1>/dev/null 2>&1
modules-k8s-gen: $(MODULE_ROOTS)
else
modules-k8s-gen:
endif

.PHONY: framework-api-gen
FRAMEWORK_API_PROTOS := $(shell find ./framework -name "*.proto")
.PHONY: $(FRAMEWORK_API_PROTOS)
$(FRAMEWORK_API_PROTOS):
	$(eval input_dir := $(shell dirname $@))
	@echo "generate framework proto api from $@"
	@protoc -I=$(input_dir) \
		-I=$(shell go env GOPATH)/src \
		--go_out=$(input_dir) \
		--go_opt=paths=source_relative \
		$@
framework-api-gen: $(FRAMEWORK_API_PROTOS)

# Generate code
.PHONY: generate-module generate-framework
ifeq ($(IN_CONTAINER),1)
generate-module: modules-api-gen modules-k8s-gen
generate-framework: framework-api-gen
generate-all: generate-module generate-framework
else
generate-module:
	@docker run --rm \
		--env IN_CONTAINER=1 \
		--env MODULES_ROOT=$(MODULES_ROOT) \
		--env MODULES="$(MODULES)" \
		-v $(root_dir):/workspaces/slime \
		--workdir /workspaces/slime \
		--user $(shell id -u):$(shell id -g) \
		$(IMG)  \
		make $@

generate-framework:
	@docker run --rm \
		--env IN_CONTAINER=1 \
		-v $(root_dir):/workspaces/slime \
		--workdir /workspaces/slime \
		--user $(shell id -u):$(shell id -g) \
		$(IMG)  \
		make $@

generate-all:
	@docker run --rm \
		--env IN_CONTAINER=1 \
		--env MODULES_ROOT=$(MODULES_ROOT) \
		--env MODULES="$(MODULES)" \
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

MODULE_NAME?=
.PHONY: new-module
new-module:
	bash bin/gen_module.sh $(MODULE_NAME)
