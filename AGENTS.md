# Agents

## Dev setup

```bash
make setup    # verify tooling, install protoc plugins to $(go env GOPATH)/bin
make up       # build and start full stack (docker compose)
make down     # stop and remove volumes
```

Windows: run everything inside WSL2.

Required tools: go, docker, protoc. Optional: k6, jq, golangci-lint.

## Building

```bash
make build    # go build ./...
make proto    # regenerate gRPC from proto/fraud/v1/*.proto and proto/ledger/v1/*.proto
```

All services share one Dockerfile (`deploy/docker/Dockerfile`) and compile via `SERVICE=<name> go build ./cmd/<name>`.

Services embed migrations via `//go:embed *.sql` in the migrations package — they migrate on startup.

## Linting

```bash
golangci-lint run ./...
```

Configured in `.golangci.yml`. Excludes gosec, prealloc, errcheck in test files.

## Services

| Service | Port | Notes |
|---------|------|-------|
| gateway | 8080 | HTTP edge, rate limiting, SSE |
| ledger | 9001 | gRPC, money movement, outbox |
| fraud | 9002 | gRPC, scoring FSM, tree inference |
| investigator | - | Kafka consumer, MCP server |

Metrics: gateway:8080/metrics, ledger:9101, fraud:9102, investigator:9103.

Run investigator as MCP server: `go run ./cmd/investigator --mcp`

## Testing

No unit tests exist yet (`**/*_test.go` not found). `make test` and `make test-integration` referenced in README are not in the Makefile.

Load test: `make load-test` (requires k6).

## Demo commands

```bash
make demo-approved    # clean payment
make demo-denied     # blocklisted card
make demo-review     # routes to manual review
make demo-reviews    # list review queue
make demo-feed       # SSE stream of all events
make demo-stream PAYMENT_ID=<id>  # SSE stream for one investigation
```

Payments require `Idempotency-Key` header.

## Env vars

Key variables (see `.env.example`):
- `ANTHROPIC_API_KEY`: enables Claude investigator; otherwise deterministic rule-based advisor runs
- `STRIPE_API_KEY`: enables real PSP calls in test mode
- `GEO_PROVIDER`: set `ipapi` for real IP geolocation, default `static` needs no network
- `SLACK_WEBHOOK_URL`, `RESEND_API_KEY`: notification integrations

## Database

Single Postgres with pgvector. Inspect via:
```bash
docker compose exec postgres psql -U postgres -d payments
```

Key tables: `payments`, `ledger_entries`, `reviews`, `outbox`, `fraud_patterns`.

Outbox relay publishes to Redpanda (Kafka API). Monitor with:
```bash
docker compose exec redpanda rpk topic list
```

## Common issues

| Symptom | Cause |
|---------|-------|
| All payments go to manual_review with degraded=true | fraud service down or slow |
| Outbox count grows | redpanda unreachable from ledger |
| Reviews stuck in pending | investigator not consuming |
| 429 responses | rate limiter (50 req/s per IP, expected) |

## Key files

- `proto/fraud/v1/fraud.proto`, `proto/ledger/v1/ledger.proto`: service definitions
- `migrations/*.sql`: schema (embedded at build)
- `deploy/monitoring/grafana/dashboards/`: Grafana dashboards (edit here, not in UI)
- `docs/architecture.md`: system design and failure policy
- `docs/adr/`: architectural decision records

## Code conventions

### Architecture
- Use cases depend only on interfaces; infrastructure implements them.
- Read `docs/adr/` to understand service design and decisions before modifying core logic.

### Extraction
- Extract to a function only when there is real use, not just because two places have similar code.

### Comments
- Use comments when the code purpose cannot be clearly shown from the code itself.

### Errors
- Use sentinel errors (`errors.New`) compared with `errors.Is`, never `==`.

### Generics
- Use generics only when the code is genuinely domain-independent.
