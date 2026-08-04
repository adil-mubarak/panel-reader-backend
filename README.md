# Panel Reader

Local-first CBZ, CBR, and PDF reader with a Go backend and React frontend.

Reader modes include animated guided frames, single pages, and continuous vertical book scrolling.

PDF import requires Poppler's `pdftocairo` command:

```sh
sudo apt install poppler-utils
```

CBR extraction is implemented in Go and does not require an external command.

## Development

Run the API:

```sh
make run backend
```

Run the API under Delve and attach a debugger to port `2345`:

```sh
make debug backend
```

For VS Code, select `Launch Panel Reader backend` and press `F5`. VS Code starts Delve itself, so `make debug backend` is not needed when using this launch configuration.

Run the frontend in another terminal:

```sh
make run frontend
```

Open `http://localhost:5173`. Runtime data is stored under `storage/` by default.

## Verification

```sh
cd backend && go test ./...
cd frontend && npm run build
```
