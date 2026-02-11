.PHONY: build frontend backend run dev clean test

VERSION := 2.0.0
BINARY := bekci
BUILD_DIR := bin
FRONTEND_DIR := frontend
EMBED_DIR := cmd/bekci/frontend_dist

# Full build: frontend then backend
build: frontend backend

frontend:
	cd $(FRONTEND_DIR) && npm install && npm run build
	rm -rf $(EMBED_DIR)
	cp -r $(FRONTEND_DIR)/dist $(EMBED_DIR)

backend:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY) ./cmd/bekci

# Build and run
run: build
	./$(BUILD_DIR)/$(BINARY)

# Dev mode: run backend without embedded frontend (use Vite dev server separately)
dev:
	@mkdir -p $(EMBED_DIR)
	@touch $(EMBED_DIR)/.gitkeep
	CGO_ENABLED=1 go run -ldflags "-X main.version=$(VERSION)-dev" ./cmd/bekci

clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(EMBED_DIR)
	@mkdir -p $(EMBED_DIR)
	@touch $(EMBED_DIR)/.gitkeep

test:
	go test -v ./...
