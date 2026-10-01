# CDN Simulator

A distributed CDN simulator built in Go demonstrating edge caching, request routing, cache invalidation, and observability.

## Architecture

```
                    ┌─────────────┐
                    │   Client    │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Router    │  (Geo/Latency-based routing)
                    │  (Layer 7)  │
                    └──────┬──────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │ Edge US  │    │ Edge EU  │    │ Edge AP  │
    │  East    │    │  West    │    │  South   │
    └────┬─────┘    └────┬─────┘    └────┬─────┘
         │               │               │
         └───────────────┼───────────────┘
                         ▼
                  ┌────────────┐
                  │   Redis    │  (Distributed Cache)
                  │  Cluster   │
                  └─────┬──────┘
                        │
                  ┌─────▼──────┐
                  │  Origin    │
                  │  Server    │
                  └────────────┘
```

## Components

| Component | Port | Description |
|-----------|------|-------------|
| Router | 80 | L7 load balancer with geo/latency routing |
| Edge (US-East) | 8081 | Cache node for US East region |
| Edge (US-West) | 8082 | Cache node for US West region |
| Edge (EU-West) | 8083 | Cache node for EU West region |
| Origin | 9090 | Backend origin server |
| Prometheus | 9090 | Metrics collection |
| Grafana | 3000 | Dashboards (admin/admin) |

## Features

- **Multi-region edge caching** with Redis backend
- **Cache invalidation** by key, tags, or wildcard patterns
- **Stale-while-revalidate** for improved latency
- **Cache warming** for pre-populating content
- **Compression** (Brotli/Gzip) at edge
- **Health checks** with automatic failover
- **Latency-based routing** to nearest healthy edge
- **Prometheus metrics** + Grafana dashboards
- **CLI tool** for cache management

## Quick Start

```bash
# Start all services
cd deployments
docker-compose up -d

# Verify services
curl http://localhost/health
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health
```

## Usage

### Basic Request (via Router)

```bash
curl http://localhost/api/v1/assets/logo.png
```

### Direct Edge Access

```bash
curl http://localhost:8081/api/v1/assets/logo.png
```

### Cache Invalidation (CLI)

```bash
# Build CLI
go build -o cdnctl ./cmd/cli

# Purge by keys
./cdnctl purge /api/v1/assets/logo.png /api/v1/assets/banner.png

# Purge by tags
./cdnctl purge-tags product:123 category:electronics

# Purge by pattern
./cdnctl purge-pattern "/api/v1/assets/*"

# View stats
./cdnctl stats

# Warm cache
./cdnctl warm /api/v1/assets/logo.png /api/v1/assets/banner.png
```

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/*` | Serve content (cached) |
| POST | `/api/v1/purge` | Purge by keys (requires `X-API-Key`) |
| POST | `/api/v1/purge/tags` | Purge by tags (requires `X-API-Key`) |
| POST | `/api/v1/warm` | Warm cache (requires `X-API-Key`) |

Admin endpoints are disabled unless `EDGE_ADMIN_API_KEY` is set.

### Purge by Keys

```bash
curl -X POST http://localhost:8081/api/v1/purge \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $EDGE_ADMIN_API_KEY" \
  -d '{"keys": ["/api/v1/assets/logo.png", "/api/v1/assets/banner.png"]}'
```

### Purge by Tags

```bash
curl -X POST http://localhost:8081/api/v1/purge/tags \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $EDGE_ADMIN_API_KEY" \
  -d '{"tags": ["product:123", "category:electronics"]}'
```

### Warm Cache

```bash
curl -X POST http://localhost:8081/api/v1/warm \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $EDGE_ADMIN_API_KEY" \
  -d '{"urls": ["/api/v1/assets/logo.png", "/api/v1/assets/banner.png"]}'
```

## Configuration

Edit `config.yaml` or use environment variables:

```yaml
edge:
  region: "us-east-1"
  port: 8080
  redis_addr: "localhost:6379"
  capacity: 10000

cache:
  default_ttl: 300           # 5 minutes
  max_ttl: 86400             # 24 hours
  stale_while_revalidate: 60 # 1 minute
  enable_tags: true
  enable_stale: true

router:
  strategy: "latency"        # latency | geo | round_robin
```

Security-related environment variables:

| Variable | Description |
|----------|-------------|
| `EDGE_ADMIN_API_KEY` | API key for `/api/v1/purge`, `/purge/tags`, `/warm`. Admin API is disabled if unset. |
| `REDIS_PASSWORD` | Redis password (compose default: `cdnsim-local-dev` — change in production). |
| `GRAFANA_ADMIN_PASSWORD` | Grafana admin password (compose default: `cdnsim-grafana-change-me` — change in production). |

## Metrics

Key metrics exposed at `/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `cdn_requests_total` | Counter | Total requests by region/status/method |
| `cdn_request_duration_seconds` | Histogram | Request latency |
| `cdn_cache_hits_total` | Counter | Cache hits (hit/stale) |
| `cdn_cache_misses_total` | Counter | Cache misses |
| `cdn_origin_requests_total` | Counter | Origin requests |
| `cdn_origin_latency_seconds` | Histogram | Origin latency |
| `cdn_bandwidth_bytes_total` | Counter | Bandwidth in/out |
| `cdn_edge_load` | Gauge | Edge utilization |
| `cdn_cache_size_bytes` | Gauge | Cache memory usage |

## Testing

### Unit Tests

```bash
go test ./...          # all tests
go test -v ./internal/cache/    # cache tests (uses miniredis, no Redis server needed)
go test -v ./internal/routing/  # routing tests
go test -cover ./...   # with coverage
```

Covered behavior:
- **routing** — edge selection (least-loaded, region preference, unhealthy/full skip), origin selection (health, weight fallback), register/deregister
- **cache** — set/get, TTL expiry, stale-while-revalidate, delete, invalidation by tags & pattern, hit/miss stats

### CI

Setiap push ke `main` / pull request menjalankan GitHub Actions (`.github/workflows/ci.yml`):

1. **test job** — `go vet`, `go build`, `go test -race` + coverage report
2. **docker job** — validasi compose, build semua image, smoke test (health check + verifikasi MISS → HIT di CI)

Lihat badge status di halaman repo.

### Load Test

Stack harus sudah jalan (`docker compose up -d`), lalu:

```powershell
# Windows
.\scripts\loadtest.ps1 -Requests 500 -UniquePaths 5
```

```bash
# Linux/macOS (butuh hey: go install github.com/rakyll/hey@latest)
hey -n 500 -c 20 http://localhost/api/v1/loadtest
curl -s http://localhost:8081/metrics | grep cdn_cache
```

Contoh hasil (`-Requests 500 -UniquePaths 5`):

```
==========================================
 Load Test Results
==========================================
 Requests      : 500
 Unique paths  : 5
 Errors        : 0
 HIT / MISS    : 495 / 5
 Client ratio  : 99%
 Metrics ratio : 98.7%
 Total time    : 1.17s
 Throughput    : 427.3 req/s
 Avg latency   : 2.3 ms
 P95 latency   : 3.1 ms
==========================================
```

> 5 path unik × 1 request pertama MISS = 5 MISS, sisanya HIT → membuktikan edge cache bekerja.


## Project Structure

```
cdn-simulator/
├── cmd/
│   ├── edge/      # Edge cache server
│   ├── origin/    # Origin server
│   ├── router/    # L7 router/load balancer
│   └── cli/       # Management CLI
├── internal/
│   ├── cache/     # Redis cache implementation
│   ├── config/    # Configuration management
│   ├── metrics/   # Prometheus metrics
│   └── routing/   # Request routing logic
├── pkg/
│   ├── types/     # Shared types
│   └── utils/     # Utilities
├── deployments/   # Docker, K8s, Prometheus, Grafana
└── tests/         # Integration tests
```

## Extending

### Add New Edge Region

1. Add service to `docker-compose.yml`
2. Configure region in `config.yaml`
3. Update router's geo database

### Custom Routing Strategy

Implement `Router` interface in `internal/routing/`:

```go
type CustomRouter struct { ... }
func (r *CustomRouter) SelectEdge(req *types.Request) (*types.EdgeNode, error) { ... }
```

### Add Cache Backend

Implement `Cache` interface in `internal/cache/`:

```go
type MemcachedCache struct { ... }
func (c *MemcachedCache) Get(ctx context.Context, key types.CacheKey) (*types.CacheEntry, error) { ... }
```

## License

MIT