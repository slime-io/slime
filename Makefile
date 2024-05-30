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
	controller-gen crd:ignoreUnexportedFields=true paths="./api/..." output:crd:dir="./charts/crds" 1>/dev/null 2>&1; \
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
		--golang-jsonshim_out=$(input_dir) \
		--golang-jsonshim_opt=paths=source_relative \
		$@
framework-api-gen: $(FRAMEWORK_API_PROTOS)
gen-slimeboot-crd:
	@echo "generate slime-boot crd"
	@pushd ./framework 1>/dev/null 2>&1; \
	controller-gen crd:ignoreUnexportedFields=true paths="./apis/config/..." output:crd:dir="./charts/crds" 1>/dev/null 2>&1; \
	popd 1>/dev/null 2>&1

.PHONY: format-go
format-go:
	go list -f '{{.Dir}}/...' -m | xargs golangci-lint run --fix -c ./.golangci-format.yaml

.PHONY: lint-go
lint-go:
	go list -f '{{.Dir}}/...' -m | xargs golangci-lint run -c ./.golangci.yaml

TEST_K8S_VERSION ?= 1.26.0
.PHONY: modules-test
modules-test:
	$(eval BIN_ASSETS:=$(shell setup-envtest --bin-dir=./testdata/bin -p path use $(TEST_K8S_VERSION) --os $(shell go env GOOS) --arch $(shell go env GOARCH)))
	@export KUBEBUILDER_ASSETS=$${PWD}/$(BIN_ASSETS) && \
	go test -cover ./staging/src/slime.io/slime/modules/{limiter,meshregistry,plugin}/...

# Generate code
.PHONY: generate-module generate-framework format lint test
ifeq ($(IN_CONTAINER),1)
generate-module: modules-api-gen modules-k8s-gen
generate-framework: framework-api-gen gen-slimeboot-crd
generate-all: generate-module generate-framework
format: format-go
lint: lint-go
test: modules-test
else
generate-module generate-all:
	@$(DOCKER_RUN) \
		--env MODULES_ROOT=$(MODULES_ROOT) \
		--env MODULES="$(MODULES)" \
		$(IMG)  \
		make $@

generate-framework:
	@$(DOCKER_RUN) \
		$(IMG)  \
		make $@

format lint:
	@$(DOCKER_RUN) \
		--env GOLANGCI_LINT_CACHE=/go/.cache \
		$(IMG)  \
		make $@

test:
	@$(DOCKER_RUN) \
		$(IMG)  \
		make $@
endif

.PHONY: shell
ifeq ($(IN_CONTAINER),1)
shell:
	@echo "already in a container"
else
shell:
	@$(DOCKER_RUN) -it \
		$(IMG)  \
		bash
endif

DOCKER_RUN := docker run --rm \
				-v $(root_dir):/workspaces/slime \
				--workdir /workspaces/slime \
				--user $(shell id -u):$(shell id -g) \
				--env IN_CONTAINER=1 \

MODULE_NAME?=
.PHONY: new-module
new-module:
	bash bin/gen_module.sh $(MODULE_NAME)
