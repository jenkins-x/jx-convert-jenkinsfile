NAME := jx-convert-jenkinsfile
# TODO: Switch to jenkins-x
ORG := abayer
ORG_REPO := $(ORG)/$(NAME)
RELEASE_ORG_REPO := $(ORG_REPO)
ROOT_PACKAGE := github.com/$(ORG_REPO)
MAIN_SRC_FILE=cmd/jx-convert-jenkinsfile/jx-convert-jenkinsfile.go
REV := $(shell git rev-parse --short HEAD 2> /dev/null || echo 'unknown')

# set dev version unless VERSION is explicitly set via environment
VERSION ?= $(shell echo "$$(git for-each-ref refs/tags/ --count=1 --sort=-version:refname --format='%(refname:short)' 2>/dev/null)-dev+$(REV)" | sed 's/^v//')

GO := GO111MODULE=on go
GO_NOMOD :=GO111MODULE=off go
REVISION        := $(shell git rev-parse --short HEAD 2> /dev/null  || echo 'unknown')
BRANCH     := $(shell git rev-parse --abbrev-ref HEAD 2> /dev/null  || echo 'unknown')
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)

GO_VERSION := $(shell $(GO) version | sed -e 's/^[^0-9.]*\([0-9.]*\).*/\1/')
GOTEST := $(GO) test

# Make does not offer a recursive wildcard function, so here's one:
rwildcard=$(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

GO_DEPENDENCIES := $(call rwildcard,pkg/,*.go) $(call rwildcard,cmd/jx-convert-jenkinsfile/,*.go)

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BUILD_DIR ?= ./bin

BUILDFLAGS := -ldflags \
  " -X $(ROOT_PACKAGE)/version.Version='$(VERSION)'\
    -X $(ROOT_PACKAGE)/version.Revision='$(REVISION)'\
    -X $(ROOT_PACKAGE)/version.Branch='$(BRANCH)'\
    -X $(ROOT_PACKAGE)/version.BuildDate='$(BUILD_DATE)'\
    -X $(ROOT_PACKAGE)/version.GoVersion='$(GO_VERSION)'"

all: test $(GOOS)-build

check: fmt test

.PHONY: build
build: $(GO_DEPENDENCIES)
	CGO_ENABLED=0 $(GO) build $(BUILDFLAGS) -o $(BUILD_DIR)/$(NAME) $(MAIN_SRC_FILE)

install: $(GO_DEPENDENCIES) ## Install the binary
	GOBIN=${GOPATH}/bin $(GO) install $(BUILDFLAGS) $(MAIN_SRC_FILE)

get-fmt-deps: ## Install test dependencies
	$(GO_NOMOD) get golang.org/x/tools/cmd/goimports

.PHONY: fmt
fmt: importfmt ## Format the code
	$(eval FORMATTED = $(shell $(GO) fmt ./...))
	@if [ "$(FORMATTED)" == "" ]; \
      	then \
      	    echo "All Go files properly formatted"; \
      	else \
      		echo "Fixed formatting for: $(FORMATTED)"; \
      	fi

.PHONY: importfmt
importfmt: get-fmt-deps
	# $(GO_NOMOD) get golang.org/x/tools/cmd/goimports
	@echo "Formatting the imports..."
	goimports -w $(GO_DEPENDENCIES)

darwin-build:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build $(BUILDFLAGS) -o $(BUILD_DIR)/$(NAME)-darwin $(MAIN_SRC_FILE)
	chmod +x bin/$(NAME)-darwin

linux-build:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build $(BUILDFLAGS) -o $(BUILD_DIR)/$(NAME)-linux $(MAIN_SRC_FILE)
	chmod +x bin/$(NAME)-linux

windows-build:
	CGO_ENABLED=0 GOARCH=amd64 GOOS=windows go build $(BUILDFLAGS) -o $(BUILD_DIR)/$(NAME)-windows.exe $(MAIN_SRC_FILE)

.PHONY: test
test:
	$(GOTEST) -failfast -short ./...

.PHONY: release
release: clean test cross
	mkdir -p release
	cp $(BUILD_DIR)/$(NAME)-* release
	gh-release checksums sha256
	gh-release create $(ORG)/$(NAME) $(VERSION) master v$(VERSION)

.PHONY: cross
cross: darwin-build linux-build windows-build

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -rf release
