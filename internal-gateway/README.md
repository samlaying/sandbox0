# Internal Gateway

Internal Gateway is the unified entry point for sandbox0, responsible for authentication, routing, rate limiting, and service coordination.

## Features

- **Authentication**: API Key and JWT authentication support
- **Authorization**: Role-based access control (RBAC)
- **Rate Limiting**: Per-team rate limiting with token bucket algorithm
- **Request Routing**: Routes requests to Manager, Procd, or Storage Proxy
- **Audit Logging**: Full request audit trail to PostgreSQL
- **Metrics**: Prometheus metrics for monitoring
- **Health Checks**: Kubernetes-compatible health endpoints

## Architecture

```
                                 ┌─────────────────┐
                                 │ Internal Gateway│
                                 │    (Port 8443)  │
                                 └────────┬────────┘
                                          │
        ┌─────────────────────────────────┼─────────────────────────────────┐
        │                                 │                                 │
        ▼                                 ▼                                 ▼
┌───────────────┐               ┌───────────────┐               ┌───────────────┐
│    Manager    │               │     Procd     │               │ Storage Proxy │
│  (Port 8080)  │               │  (Dynamic)    │               │  (Port 8081)  │
└───────────────┘               └───────────────┘               └───────────────┘
```

## Building

```bash
# Build binary
make build

# Run locally
make run-local

# Build Docker image
make docker-build
```

## Deployment

```bash
# Apply Kubernetes manifests
kubectl apply -k deploy/k8s/

# Or apply individual files
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/secret.yaml
kubectl apply -f deploy/k8s/networkpolicy.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

## Authentication

### API Key
```
Authorization: Bearer sb0_<team_id>_<random_secret>
```

### JWT (Optional)
```
Authorization: Bearer <jwt_token>
```

## Metrics

Prometheus metrics are available at `/metrics`:
- `gateway_http_requests_total` - Total HTTP requests
- `gateway_http_request_duration_seconds` - Request latency
- `gateway_proxy_requests_total` - Proxied requests
- `gateway_auth_failures_total` - Authentication failures
- `gateway_rate_limit_hits_total` - Rate limit hits

## Health Checks

- `/healthz` - Liveness probe
- `/readyz` - Readiness probe (includes database connectivity check)

