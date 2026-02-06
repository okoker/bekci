.PHONY: build run clean test

VERSION := 0.1.0
BINARY := bekci
BUILD_DIR := bin

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/bekci

run: build
	./$(BUILD_DIR)/$(BINARY)

clean:
	rm -rf $(BUILD_DIR)

test:
	go test -v ./...

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/
