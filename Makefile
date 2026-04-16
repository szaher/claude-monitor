.PHONY: build test clean install

VERSION ?= dev
BINARY_NAME = claude-monitor
BUILD_DIR = bin

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/claude-monitor

test:
	go test -v ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

run: build
	$(BUILD_DIR)/$(BINARY_NAME)
