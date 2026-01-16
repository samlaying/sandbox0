.PHONY: proto build docker clean test

DOCKER_IMAGE=sandbox0ai/storage-proxy
VERSION?=latest

# Generate protobuf code
proto:
	mkdir -p proto/fs
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/filesystem.proto
	mv proto/*.pb.go proto/fs/

# Build binary
build: proto
	#CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/storage-proxy ./cmd/storage-proxy
	go build -o bin/storage-proxy ./cmd/storage-proxy

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

docker-push:
	docker push $(DOCKER_IMAGE):$(VERSION)

# Run tests
test:
	@GOTOOLCHAIN=go1.25.0+auto go test -v -race -cover ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf proto/fs/*.pb.go

# Install protoc dependencies
install-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Run locally (for development)
run:
	go run ./cmd/storage-proxy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run ./...

