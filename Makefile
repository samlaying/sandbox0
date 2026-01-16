.PHONY: all build build-all obuild obuild-all test test-all lint tidy vendor clean helm-update release docker-build-all docker-push-all

SERVICES := edge-gateway internal-gateway manager storage-proxy netd k8s-plugin

# Default version
VERSION ?= latest

# Colors for output
YELLOW := \033[1;33m
GREEN  := \033[1;32m
CYAN   := \033[1;36m
RESET  := \033[0m

all: build-all

# Build all services
build-all:
	@for service in $(SERVICES); do \
		printf "$(GREEN)Building $$service...$(RESET)\n"; \
		if [ -f "$$service/Makefile" ]; then \
			$(MAKE) -C $$service build || exit 1; \
		elif [ -d "$$service/cmd" ]; then \
			go build -v -o $$service/bin/$$service ./$$service/cmd/... || exit 1; \
		elif [ -f "$$service/main.go" ]; then \
			go build -v -o $$service/bin/$$service ./$$service/ || exit 1; \
		fi; \
	done

# Docker build all services
docker-build-all:
	@for service in $(SERVICES); do \
		printf "$(GREEN)Docker building $$service...$(RESET)\n"; \
		if [ "$$service" = "k8s-plugin" ]; then \
			$(MAKE) -C $$service docker IMAGE_TAG=$(VERSION) || exit 1; \
		elif [ -f "$$service/Makefile" ]; then \
			$(MAKE) -C $$service docker-build VERSION=$(VERSION) IMAGE_TAG=$(VERSION) || exit 1; \
		elif [ -f "$$service/Dockerfile" ]; then \
			docker build -t sandbox0ai/$$service:$(VERSION) $$service || exit 1; \
		fi; \
	done

# Docker push all services
docker-push-all:
	@for service in $(SERVICES); do \
		printf "$(GREEN)Docker pushing $$service...$(RESET)\n"; \
		if [ "$$service" = "k8s-plugin" ]; then \
			$(MAKE) -C $$service push-image IMAGE_TAG=$(VERSION) || exit 1; \
		elif [ -f "$$service/Makefile" ]; then \
			$(MAKE) -C $$service docker-push VERSION=$(VERSION) IMAGE_TAG=$(VERSION) || exit 1; \
		elif [ -f "$$service/Dockerfile" ]; then \
			docker push sandbox0ai/$$service:$(VERSION) || exit 1; \
		fi; \
	done

# Build specific service: make build <service>
build:
	@service="$(filter-out build build-all obuild obuild-all test test-all lint tidy vendor clean helm-update,$(MAKECMDGOALS))"; \
	if [ -z "$$service" ]; then \
		echo "Error: Please specify a service or use 'make build-all'"; \
		echo "Available services: $(SERVICES)"; \
		echo "Usage: make build <service> or make build-<service>"; \
		exit 1; \
	elif echo "$(SERVICES)" | grep -qw "$$service"; then \
		printf "$(GREEN)Building $$service...$(RESET)\n"; \
		if [ -f "$$service/Makefile" ]; then \
			$(MAKE) -C $$service build; \
		elif [ -d "$$service/cmd" ]; then \
			go build -v -o $$service/bin/$$service ./$$service/cmd/...; \
		elif [ -f "$$service/main.go" ]; then \
			go build -v -o $$service/bin/$$service ./$$service/; \
		fi; \
	else \
		echo "Error: Unknown service '$$service'"; \
		echo "Available services: $(SERVICES)"; \
		exit 1; \
	fi

test:
	@service="$(filter-out build build-all obuild obuild-all test test-all lint tidy vendor clean helm-update,$(MAKECMDGOALS))"; \
	if [ -z "$$service" ]; then \
		echo "Available services: $(SERVICES)"; \
		echo "Usage: make test <service> or make test-all"; \
		exit 1; \
	elif echo "$(SERVICES)" | grep -qw "$$service"; then \
		printf "$(CYAN)Testing $$service...$(RESET)\n"; \
		if [ -f "$$service/Makefile" ]; then \
			$(MAKE) -C $$service test; \
		elif [ -d "$$service/cmd" ]; then \
			GOTOOLCHAIN=go1.25.0+auto go test -v -race -cover ./$$service/cmd/...; \
		elif [ -f "$$service/main.go" ]; then \
			GOTOOLCHAIN=go1.25.0+auto go test -v -race -cover ./$$service/; \
		fi; \
	else \
		echo "Error: Unknown service '$$service'"; \
		echo "Available services: $(SERVICES)"; \
		exit 1; \
	fi

test-all:
	@for service in $(SERVICES); do \
		printf "$(CYAN)Testing $$service...$(RESET)\n"; \
		$(MAKE) test $$service || exit 1; \
	done

# Direct go build specific service: make obuild <service> (no Makefile delegation)
obuild:
	@service="$(filter-out build build-all obuild test lint tidy vendor clean helm-update,$(MAKECMDGOALS))"; \
	if [ -z "$$service" ]; then \
		echo "Error: Please specify a service or use 'make obuild-all'"; \
		echo "Available services: $(SERVICES)"; \
		echo "Usage: make obuild <service>"; \
		exit 1; \
	elif echo "$(SERVICES)" | grep -qw "$$service"; then \
		printf "$(GREEN)Direct go build $$service...$(RESET)\n"; \
		if [ -d "$$service/cmd" ]; then \
			go vet ./$$service/cmd/...; \
			go build -v -o $$service/bin ./$$service/cmd/...; \
		elif [ -f "$$service/main.go" ]; then \
			go vet ./$$service/; \
			go build -v -o $$service/bin ./$$service/; \
		else \
			echo "Warning: No cmd directory or main.go found for $$service"; \
		fi; \
	else \
		echo "Error: Unknown service '$$service'"; \
		echo "Available services: $(SERVICES)"; \
		exit 1; \
	fi

# Prevent make from treating service names as targets
edge-gateway internal-gateway manager storage-proxy netd k8s-plugin:
	@:

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

vendor:
	go mod vendor

clean:
	@for service in $(SERVICES); do \
		printf "$(YELLOW)Cleaning $$service...$(RESET)\n"; \
		rm -rf $$service/bin; \
	done
	rm -rf vendor

helm-update:
	@mkdir -p helm/charts
	@for service in $(SERVICES); do \
		if [ -d "$$service/chart" ]; then \
			echo "Copying chart for $$service..."; \
			rm -rf helm/charts/$$service; \
			cp -r $$service/chart helm/charts/$$service; \
		fi; \
	done

helm-clean:
	@mkdir -p helm/charts
	@for service in $(SERVICES); do \
		if [ -d "$$service/chart" ]; then \
			echo "Deleting chart for $$service..."; \
			rm -rf helm/charts/$$service; \
		fi; \
	done

# Release helm chart and git tag in one shot:
#   make release VERSION=v0.1.0
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required. Usage: make release VERSION=v0.1.0"; \
		exit 2; \
	fi
	@bash release.sh "$(VERSION)"
