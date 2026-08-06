.PHONY: help run debug debug-server backend backend-ai frontend ai ai-build ai-local-build ai-local ai-health ai-venv dataset-prepare dataset-validate train-ai test build

DATASET ?= ai-service/dataset.yaml
DATASET_SOURCE ?=
DATASET_OUTPUT ?= datasets/comic-panels
BASE ?= yolo11n-seg.pt
EPOCHS ?= 100
IMAGE_SIZE ?= 1280
BATCH ?= 8
DEVICE ?=
FRACTION ?= 1.0

help:
	@printf '%s\n' \
		'make run backend   Run the Go API' \
		'make run frontend  Run the frontend development server' \
		'make run ai        Run the Python panel detector in Docker' \
		'make run backend-ai Run the Go API with the AI detector enabled' \
		'make ai-local-build Build the local Ultralytics AI image' \
		'make ai-local       Run local checkpoint inference in Docker' \
		'make ai-health      Query the AI service health endpoint' \
		'make dataset-prepare DATASET_SOURCE=... DATASET_OUTPUT=...' \
		'make dataset-validate DATASET=path/to/data.yaml' \
		'make train-ai DATASET=... BASE=... EPOCHS=100 IMAGE_SIZE=1280 BATCH=8 DEVICE=0 FRACTION=1.0' \
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

ai-local-build:
	docker compose -f compose.yaml -f compose.local.yaml build panel-ai

ai-local:
	PANEL_AI_UID=$$(id -u) PANEL_AI_GID=$$(id -g) docker compose -f compose.yaml -f compose.local.yaml up panel-ai

ai-health:
	curl --fail --show-error http://127.0.0.1:8090/health

ai-venv: ai-service/.venv/.requirements-local.stamp

ai-service/.venv/.requirements-local.stamp: ai-service/requirements.txt ai-service/requirements-local.txt
	python3 -m venv ai-service/.venv
	ai-service/.venv/bin/python -m pip install -r ai-service/requirements-local.txt
	@touch ai-service/.venv/.requirements-local.stamp

dataset-validate: ai-venv
	ai-service/.venv/bin/python ai-service/validate_dataset.py --data "$(DATASET)"

dataset-prepare: ai-venv
	ai-service/.venv/bin/python ai-service/prepare_roboflow_dataset.py --source "$(DATASET_SOURCE)" --output "$(DATASET_OUTPUT)"

train-ai: ai-venv
	ai-service/.venv/bin/python ai-service/train.py --data "$(DATASET)" --base "$(BASE)" --epochs "$(EPOCHS)" --image-size "$(IMAGE_SIZE)" --batch "$(BATCH)" --device "$(DEVICE)" --fraction "$(FRACTION)"

frontend: frontend/node_modules
	npm --prefix frontend run dev

frontend/node_modules: frontend/package.json frontend/package-lock.json
	npm --prefix frontend install
	@touch frontend/node_modules

test:
	go -C backend test ./...

build: frontend/node_modules
	npm --prefix frontend run build
