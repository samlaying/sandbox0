# E2B Code Interpreter SDK - Backend API Endpoints

This document lists all backend API endpoints called by the e2b-code-interpreter Python SDK, grouped by host and port.

## Overview

The E2B SDK communicates with three distinct backend services:

| Service | Default Host | Port | Protocol |
|---------|-------------|------|----------|
| **Control Plane API** | `api.e2b.app` | 443 (HTTPS) | HTTP/JSON |
| **Envd API** (Sandbox Runtime) | `{port}-{sandbox_id}.{sandbox_domain}` | `49983` | HTTP + Connect RPC |
| **Jupyter API** (Code Interpreter) | `{port}-{sandbox_id}.{sandbox_domain}` | `49999` | HTTP/JSON |

---

## 1. Control Plane API

**Host**: `https://api.e2b.app` (production) or `http://localhost:3000` (debug mode)
**Port**: `443` (HTTPS) or `3000` (HTTP debug)
**Authentication**: API Key via `E2B-API-Key` header

### Sandbox Management

| Method | Endpoint | Description | SDK Method |
|--------|----------|-------------|------------|
| `POST` | `/sandboxes` | Create a new sandbox | `Sandbox.create()` |
| `GET` | `/sandboxes` | List all sandboxes | `Sandbox.list()` |
| `GET` | `/v2/sandboxes` | List sandboxes (v2) | `Sandbox.list()` (paginated) |
| `GET` | `/sandboxes/{sandbox_id}` | Get sandbox info | `Sandbox.get_info()` |
| `DELETE` | `/sandboxes/{sandbox_id}` | Kill/terminate sandbox | `Sandbox.kill()` |
| `POST` | `/sandboxes/{sandbox_id}/connect` | Connect to existing sandbox | `Sandbox.connect()` |
| `POST` | `/sandboxes/{sandbox_id}/pause` | Pause sandbox | `Sandbox.beta_pause()` |
| `POST` | `/sandboxes/{sandbox_id}/resume` | Resume paused sandbox | `Sandbox.connect()` (auto-resume) |
| `POST` | `/sandboxes/{sandbox_id}/timeout` | Update sandbox timeout | `Sandbox.set_timeout()` |
| `POST` | `/sandboxes/{sandbox_id}/refreshes` | Refresh sandbox session | Internal use |
| `GET` | `/sandboxes/{sandbox_id}/logs` | Get sandbox logs | Internal use |
| `GET` | `/sandboxes/{sandbox_id}/metrics` | Get sandbox metrics | `Sandbox.get_metrics()` |
| `GET` | `/sandboxes/metrics` | Get all sandboxes metrics | Internal use |

### Template Management

| Method | Endpoint | Description | SDK Method |
|--------|----------|-------------|------------|
| `GET` | `/templates` | List templates | `Template.list()` |
| `POST` | `/templates` | Create template | `Template.create()` |
| `POST` | `/v2/templates` | Create template (v2) | `Template.create()` |
| `POST` | `/v3/templates` | Create template (v3) | `Template.create()` |
| `GET` | `/templates/{template_id}` | Get template details | `Template.get()` |
| `PATCH` | `/templates/{template_id}` | Update template | `Template.update()` |
| `DELETE` | `/templates/{template_id}` | Delete template | `Template.delete()` |
| `POST` | `/templates/{template_id}` | Start template build | `Template.build()` |
| `GET` | `/templates/{template_id}/files/{hash}` | Get template files | Internal use |
| `POST` | `/templates/{template_id}/builds/{build_id}` | Control template build | `Template.build()` |
| `GET` | `/templates/{template_id}/builds/{build_id}/status` | Get build status | `Template.build()` |
| `POST` | `/v2/templates/{template_id}/builds/{build_id}` | Control build (v2) | Internal use |

---

## 2. Envd API (Sandbox Runtime)

**Host**: `http://49983-{sandbox_id}.{sandbox_domain}` (HTTPS in production, HTTP in debug)
**Port**: `49983`
**Authentication**: `X-Access-Token` header (envd access token)
**Custom Headers**:
- `E2b-Sandbox-Id`: Sandbox ID
- `E2b-Sandbox-Port`: `49983`
- `Keepalive-Ping-Interval`: `50` (seconds)

### REST Endpoints

| Method | Endpoint | Description | SDK Method |
|--------|----------|-------------|------------|
| `GET` | `/health` | Health check | `Sandbox.is_running()` |
| `GET` | `/files` | Read file content | `sandbox.files.read()` |
| `POST` | `/files` | Write file(s) - multipart/form-data | `sandbox.files.write()`, `sandbox.files.write_files()` |

### Connect RPC Endpoints (gRPC over HTTP)

The SDK uses [Connect RPC](https://connect.build/docs) for streaming operations.

#### Filesystem Operations (`filesystem.Filesystem`)

| RPC Method | Connect Endpoint | Description | SDK Method |
|------------|------------------|-------------|------------|
| `Stat` | `POST /filesystem.Filesystem/Stat` | Get file/directory info | `sandbox.files.get_info()`, `sandbox.files.exists()` |
| `MakeDir` | `POST /filesystem.Filesystem/MakeDir` | Create directory | `sandbox.files.make_dir()` |
| `Move` | `POST /filesystem.Filesystem/Move` | Rename/move file | `sandbox.files.rename()` |
| `ListDir` | `POST /filesystem.Filesystem/ListDir` | List directory contents | `sandbox.files.list()` |
| `Remove` | `POST /filesystem.Filesystem/Remove` | Remove file/directory | `sandbox.files.remove()` |
| `WatchDir` | `POST /filesystem.Filesystem/WatchDir` | Watch directory (stream) | `sandbox.files.watch_dir()` (internal) |
| `CreateWatcher` | `POST /filesystem.Filesystem/CreateWatcher` | Create directory watcher | `sandbox.files.watch_dir()` |
| `GetWatcherEvents` | `POST /filesystem.Filesystem/GetWatcherEvents` | Get watcher events | `WatchHandle` events |
| `RemoveWatcher` | `POST /filesystem.Filesystem/RemoveWatcher` | Remove watcher | `WatchHandle` cleanup |

#### Process Operations (`process.Process`)

| RPC Method | Connect Endpoint | Description | SDK Method |
|------------|------------------|-------------|------------|
| `Start` | `POST /process.Process/Start` | Start command/PTY (stream) | `sandbox.commands.run()`, `sandbox.pty.create()` |
| `Connect` | `POST /process.Process/Connect` | Connect to running process (stream) | `sandbox.commands.connect()`, `sandbox.pty.connect()` |
| `List` | `POST /process.Process/List` | List running processes | `sandbox.commands.list()` |
| `SendSignal` | `POST /process.Process/SendSignal` | Send signal (kill) | `sandbox.commands.kill()`, `sandbox.pty.kill()` |
| `SendInput` | `POST /process.Process/SendInput` | Send stdin input | `sandbox.commands.send_stdin()`, `sandbox.pty.send_stdin()` |
| `Update` | `POST /process.Process/Update` | Update process (resize PTY) | `sandbox.pty.resize()` |
| `StreamInput` | `POST /process.Process/StreamInput` | Stream input (client stream) | Internal use |

---

## 3. Jupyter API (Code Interpreter)

**Host**: `http://49999-{sandbox_id}.{sandbox_domain}` (HTTPS in production, HTTP in debug)
**Port**: `49999`
**Authentication**:
- `X-Access-Token`: Envd access token (optional)
- `E2B-Traffic-Access-Token`: Traffic access token (optional)

| Method | Endpoint | Description | SDK Method |
|--------|----------|-------------|------------|
| `POST` | `/execute` | Execute code in context | `sandbox.run_code()` |
| `GET` | `/contexts` | List all code contexts | `sandbox.list_code_contexts()` |
| `POST` | `/contexts` | Create new code context | `sandbox.create_code_context()` |
| `DELETE` | `/contexts/{context_id}` | Remove/delete context | `sandbox.remove_code_context()` |
| `POST` | `/contexts/{context_id}/restart` | Restart context kernel | `sandbox.restart_code_context()` |

---

## Request/Response Examples

### Create Sandbox (Control Plane)
```bash
curl -X POST https://api.e2b.app/sandboxes \
  -H "E2B-API-Key: your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "templateID": "code-interpreter-v1",
    "timeout": 300,
    "metadata": {"key": "value"}
  }'
```

### Execute Code (Jupyter API)
```bash
curl -X POST https://49999-abc123.e2b.app/execute \
  -H "X-Access-Token: envd_token" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "print(\"Hello World\")",
    "context_id": "ctx-123"
  }'
```

### Read File (Envd API)
```bash
curl -X GET https://49983-abc123.e2b.app/files?path=/tmp/test.txt \
  -H "E2b-Sandbox-Id: abc123" \
  -H "X-Access-Token: envd_token"
```

### Start Process (Envd RPC)
```bash
curl -X POST https://49983-abc123.e2b.app/process.Process/Start \
  -H "Content-Type: application/json" \
  -H "E2b-Sandbox-Id: abc123" \
  -d '{
    "process": {
      "cmd": "/bin/bash",
      "args": ["-l", "-c", "echo hello"]
    }
  }'
```

---

## SDK to Endpoint Mapping

### `Sandbox.create()`
1. `POST /sandboxes` (Control Plane) - Create sandbox

### `Sandbox.kill()`
1. `DELETE /sandboxes/{sandbox_id}` (Control Plane)

### `Sandbox.connect()`
1. `POST /sandboxes/{sandbox_id}/connect` (Control Plane)

### `sandbox.run_code()`
1. `POST /execute` (Jupyter API :49999)

### `sandbox.files.read()`
1. `GET /files` (Envd API :49983)

### `sandbox.files.write()`
1. `POST /files` (Envd API :49983)

### `sandbox.files.list()`
1. `POST /filesystem.Filesystem/ListDir` (Envd RPC :49983)

### `sandbox.files.make_dir()`
1. `POST /filesystem.Filesystem/MakeDir` (Envd RPC :49983)

### `sandbox.files.remove()`
1. `POST /filesystem.Filesystem/Remove` (Envd RPC :49983)

### `sandbox.files.rename()`
1. `POST /filesystem.Filesystem/Move` (Envd RPC :49983)

### `sandbox.files.watch_dir()`
1. `POST /filesystem.Filesystem/CreateWatcher` (Envd RPC :49983)
2. `POST /filesystem.Filesystem/GetWatcherEvents` (Envd RPC :49983) - streaming

### `sandbox.commands.run()`
1. `POST /process.Process/Start` (Envd RPC :49983) - streaming

### `sandbox.commands.kill()`
1. `POST /process.Process/SendSignal` (Envd RPC :49983)

### `sandbox.commands.list()`
1. `POST /process.Process/List` (Envd RPC :49983)

### `sandbox.pty.create()`
1. `POST /process.Process/Start` (Envd RPC :49983) - with PTY config

### `Sandbox.is_running()`
1. `GET /health` (Envd API :49983)

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `E2B_API_KEY` | API key for Control Plane | - |
| `E2B_DOMAIN` | Domain for API endpoints | `e2b.app` |
| `E2B_API_URL` | Override Control Plane URL | `https://api.e2b.app` |
| `E2B_SANDBOX_URL` | Override Sandbox URL | - |
| `E2B_DEBUG` | Enable debug mode (localhost) | `false` |

---

## Debug Mode

When `E2B_DEBUG=true`, the SDK uses:
- Control Plane: `http://localhost:3000`
- Envd: `http://localhost:49983`
- Jupyter: `http://localhost:49999`

---

## References

- SDK Source: `/Users/huangzhihao/sandbox0/e2b-code-interpreter/python/e2b_code_interpreter/`
- E2B Package: `/Users/huangzhihao/sandbox0/.venv/lib/python3.14/site-packages/e2b/`
- Protocol Definitions: `.proto` files in `envd/` subdirectory
