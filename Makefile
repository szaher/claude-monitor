.PHONY: build test clean install

VERSION ?= dev
BINARY_NAME = claude-monitor
BUILD_DIR = bin

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -tags fts5 -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/claude-monitor

test:
	CGO_ENABLED=1 go test -tags fts5 -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run: build
	$(BUILD_DIR)/$(BINARY_NAME)
