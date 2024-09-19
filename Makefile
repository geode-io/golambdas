# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GOBIN ?= $(shell which go)

.PHONY: all
all: build

.PHONY: generate
generate: ## Generate code.
	$(GOBIN) generate ./...

.PHONY: lint
lint: ## Lints the project, logging any warnings or errors without modifying any files.
	$(GOBIN) run github.com/golangci/golangci-lint/cmd/golangci-lint run ./...

.PHONY: fmt
fmt: ## Reformat all code with the go fmt command.
	$(GOBIN) fmt ./...

.PHONY: vet
vet: ## Run vet on all code with the go vet command.
	$(GOBIN) vet ./...

##@ Tests

.PHONY: test
test: ## Run all unit tests.
	$(GOBIN) test -v -race ./...

.PHONY: test/short
test/short: ## Run all unit tests in short-mode.
	$(GOBIN) test -v -race -short ./...

##@ Misc.

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help: ## Display usage help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9\/-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)