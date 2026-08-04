.PHONY: help run debug debug-server backend frontend test build

help:
	@printf '%s\n' \
		'make run backend   Run the Go API' \
		'make run frontend  Run the frontend development server' \
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

frontend: frontend/node_modules
	npm --prefix frontend run dev

frontend/node_modules: frontend/package.json frontend/package-lock.json
	npm --prefix frontend install
	@touch frontend/node_modules

test:
	go -C backend test ./...

build: frontend/node_modules
	npm --prefix frontend run build
