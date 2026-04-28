HOSTNAME    = registry.terraform.io
NAMESPACE   = jackyoung
NAME        = zarf
BINARY      = terraform-provider-$(NAME)
VERSION     = 0.1.0
OS_ARCH    := $(shell go env GOOS)_$(shell go env GOARCH)
PLUGIN_DIR  = ~/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)/$(OS_ARCH)

default: build

.PHONY: build
build:
	go build -o $(BINARY) -ldflags "-X main.version=$(VERSION)"

.PHONY: install
install: build
	mkdir -p $(PLUGIN_DIR)
	mv $(BINARY) $(PLUGIN_DIR)/$(BINARY)

.PHONY: test
test:
	go test ./... -v -timeout 120s

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	go fmt ./...
	gofmt -s -w .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)
