.PHONY: dev down build push release deploy undeploy lint test

REGISTRY ?= ghcr.io/xmaeltht
IMAGE_BACKEND ?= $(REGISTRY)/trading-watchlist-backend
SHA ?= $(shell git rev-parse --short HEAD)

# ─── Dev ──────────────────────────────────────────────────
dev:
	podman compose up --build

down:
	podman compose down

# ─── Build ────────────────────────────────────────────────
build:
	podman build -t $(IMAGE_BACKEND):$(SHA) -t $(IMAGE_BACKEND):latest ./backend

push: build
	podman push $(IMAGE_BACKEND):$(SHA)
	podman push $(IMAGE_BACKEND):latest

release: push
	@echo "Pushed $(IMAGE_BACKEND):$(SHA) and :latest"

# ─── Backend ─────────────────────────────────────────────
run:
	cd backend && go run ./cmd/server

test:
	cd backend && go test ./...

lint:
	cd backend && golangci-lint run ./...

# ─── Helm ────────────────────────────────────────────────
helm-deps:
	helm dependency update helm/trading-watchlist

lint-helm:
	helm lint helm/trading-watchlist

deploy:
	helm upgrade --install trading-watchlist helm/trading-watchlist \
		--namespace maelkloud \
		--create-namespace \
		--set backend.image.tag=$(SHA) \
		--set secrets.jwtSecret=$(JWT_SECRET) \
		--set secrets.polygonApiKey=$(POLYGON_API_KEY) \
		--set secrets.finnhubApiKey=$(FINNHUB_API_KEY)

undeploy:
	helm uninstall trading-watchlist -n maelkloud
