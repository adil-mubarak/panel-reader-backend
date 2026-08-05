.PHONY: help run debug debug-server backend backend-ai frontend ai ai-build test build

help:
	@printf '%s\n' \
		'make run backend   Run the Go API' \
		'make run frontend  Run the frontend development server' \
		'make run ai        Run the Python panel detector in Docker' \
		'make run backend-ai Run the Go API with the AI detector enabled' \
		'make debug backend Run the Go API with Delve on port 2345' \
		'make test          Run backend tests' \
		'make build         Build the frontend'

# Allows the command form requested by the project: make run <service>.
run:
	@:

debug:
ifeq ($(filter backend,$(MAKECMDGOALS)),backend)
	$(MAKE) --directory=backend --file=../Makefile debug-server
else
	@printf '%s\n' 'Usage: make debug backend'
endif

debug-server:
	go build -gcflags='panel-reader/backend/...=-N -l' -o ../storage/panel-reader-debug ./cmd/server
	dlv exec ../storage/panel-reader-debug --headless --listen=:2345 --api-version=2 --accept-multiclient --continue

backend:
ifeq ($(filter debug,$(MAKECMDGOALS)),debug)
	@:
else
	go -C backend run ./cmd/server
endif

backend-ai:
	PANEL_READER_AI_URL=http://127.0.0.1:8090 PANEL_READER_AI_STORAGE_ROOT=/data go -C backend run ./cmd/server

ai:
	PANEL_AI_UID=$$(id -u) PANEL_AI_GID=$$(id -g) docker compose up panel-ai

ai-build:
	docker compose build panel-ai

frontend: frontend/node_modules
	npm --prefix frontend run dev

frontend/node_modules: frontend/package.json frontend/package-lock.json
	npm --prefix frontend install
	@touch frontend/node_modules

test:
	go -C backend test ./...

build: frontend/node_modules
	npm --prefix frontend run build
