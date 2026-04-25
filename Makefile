# Makefile for local development

.PHONY: help build test lint install run-local run-docker run-k8s clean

# ============================================
# Variables
# ============================================
GO_VERSION := 1.26
IMAGE := ghcr.io/$(ORG)/orchestrator:latest
WORKER_IMAGE := ghcr.io/$(ORG)/orchestrator-worker:latest
K8S_NS := ai-orchestrator

# ============================================
# Help
# ============================================
help:
	@echo "AI Orchestrator - Development Commands"
	@echo ""
	@echo "  make install      - Install dependencies"
	@echo "  make build        - Build binaries"
	@echo "  make test         - Run tests"
	@echo "  make lint         - Run linters"
	@echo "  make run-local    - Run locally (no Docker)"
	@echo "  make run-docker   - Run with Docker"
	@echo "  make run-k8s      - Deploy to K8s"
	@echo "  make clean        - Clean builds"
	@echo ""

# ============================================
# Install / Build
# ============================================
install:
	go mod download
	go mod tidy

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/orchestrator ./cmd/server
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/worker ./cmd/worker
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/orchestrator-cli ./cmd/cli

# ============================================
# Test / Lint
# ============================================
test:
	go test -v -race ./...

lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# Format
fmt:
	go fmt ./...
	goimports -w .

# ============================================
# Run locally
# ============================================
run-local:
	go run ./cmd/orchestrator/

run-local-distributed:
	go run ./cmd/orchestrator/ --distributed

run-server:
	go run ./cmd/server/main.go -distributed

# ============================================
# CLI
# ============================================
cli-health:
	go run ./cmd/cli/main.go health -addr=http://localhost:8080

cli-run:
	go run ./cmd/cli/main.go run "$(GOAL)" -addr=http://localhost:8080

cli-list:
	go run ./cmd/cli/main.go list -addr=http://localhost:8080

cli-queue:
	go run ./cmd/cli/main.go queue -addr=http://localhost:8080

# ============================================
# Docker
# ============================================
docker-build:
	docker build -f deploy/Dockerfile --target orchestrator -t orchestrator .
	docker build -f deploy/Dockerfile --target worker -t worker:latest .

docker-run:
	docker run -d -p 8080:8080 --name orchestrator orchestrator

docker-stop:
	docker stop orchestrator || true
	docker rm orchestrator || true

# ============================================
# Docker Compose
# ============================================
compose-up:
	docker compose -f deploy/docker-compose.yml up -d

compose-down:
	docker compose -f deploy/docker-compose.yml down

compose-logs:
	docker compose -f deploy/docker-compose.yml logs -f

# ============================================
# Kubernetes
# ============================================
k8s-apply:
	kubectl apply -k deploy/k8s/

k8s-delete:
	kubectl delete -k deploy/k8s/

k8s-logs:
	kubectl logs -n $(K8S_NS) deployment/orchestrator -f

k8s-status:
	kubectl get all -n $(K8S_NS)

k8s-port-forward:
	kubectl port-forward -n $(K8S_NS) svc/orchestrator 8080:80

# ============================================
# Deploy
# ============================================
deploy: docker-build compose-up

undeploy: compose-down

# ============================================
# Clean
# ============================================
clean:
	rm -rf bin/
	go clean -cache

dist-clean: clean
	docker system prune -af