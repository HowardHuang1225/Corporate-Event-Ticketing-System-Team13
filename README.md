# Corporate Event Ticketing System

A role-based event ticketing system for company activities. The system supports event creation, employee ticket applications, QR-code check-in, HR reporting, file uploads, caching, load testing, and Kubernetes deployment.

## Features

* **JWT authentication** and role-based access control.
* **Employee flow**: browse events, apply for tickets, view/cancel personal tickets.
* **Event manager flow**: create events, publish/close events, upload assets, and check in tickets.
* **HR flow**: view event statistics, overview reports, and CSV exports.
* **PostgreSQL** for users, events, ticket types, applications, tickets, and check-ins.
* **Redis** for inventory counters, short-lived read cache, locks, and optional queue mode.
* **MinIO** for event image/PDF storage.
* **Prometheus metrics** for HTTP and ticketing behavior.
* **Docker Compose** for local development and **Kubernetes manifests** for cluster deployment.

## Tech Stack

| Layer           | Technologies                                                                                      |
| --------------- | ------------------------------------------------------------------------------------------------- |
| Frontend        | React, TypeScript, Vite, React Router, TanStack Query, Axios, Vitest, Playwright                  |
| Backend         | Go, Gin, GORM, PostgreSQL, Redis, MinIO, JWT, Prometheus client                                   |
| Infrastructure  | Docker Compose, Kubernetes, Nginx, GitHub Actions, GHCR, AKS-ready manifests                      |
| Quality / Tests | Go tests, race detector, Vitest coverage, Playwright E2E, golangci-lint, Trivy, SonarCloud config |

## Repository Structure

```text
.
├── api/                    # OpenAPI contract
├── backend/                # Go backend service
│   ├── bootstrap/          # Demo seed data
│   ├── config/             # Environment configuration
│   ├── database/           # GORM connection and schema migration
│   ├── handler/            # HTTP handlers by domain/role
│   ├── middleware/         # JWT auth and role authorization
│   ├── model/              # GORM models
│   ├── pkg/                # JWT, Redis, TOTP, MinIO helpers
│   ├── repository/         # Data access layer
│   ├── routes/             # API route registration
│   ├── scheduler/          # Event lifecycle scheduler
│   └── service/            # Business logic layer
├── frontend/               # React frontend application
│   ├── e2e/                # Playwright E2E tests
│   └── src/
│       ├── api/            # Axios client
│       ├── components/     # Shared UI components
│       ├── contexts/       # Auth state provider
│       ├── integration/    # Frontend integration tests
│       ├── pages/          # Employee, manager, and HR pages
│       └── utils/          # Frontend utilities such as TOTP
├── docs/                   # Design notes and test documentation
├── k8s/                    # Kubernetes manifests
├── load-test/              # k6 load-test scripts
├── docker-compose.yml      # Local full-stack runtime
├── docker-compose.ci.yml   # CI integration/E2E runtime
└── .env.example            # Environment template
```

## Architecture

```text
Browser
  │
  ▼
Frontend SPA / Nginx
  │  /v1 API proxy
  ▼
Go Backend API
  ├── PostgreSQL: users, events, ticket types, applications, tickets, check-ins
  ├── Redis: inventory cache, read cache, locks, queue status, Redis Stream waiting room
  ├── MinIO: uploaded event images and PDF documents
  └── /metrics: Prometheus metrics
```

The backend follows a handler-service-repository structure. Handlers process HTTP requests and role context, services implement business rules, and repositories isolate database access.

## Quick Start

### Prerequisites

* Docker and Docker Compose
* Git

### Run with Docker Compose

```bash
cp .env.example .env
docker compose up --build
```

After startup:

| Service            | URL                           |
| ------------------ | ----------------------------- |
| Frontend           | http://localhost:8080         |
| Backend API        | http://localhost:8001         |
| Health check       | http://localhost:8001/health  |
| Readiness check    | http://localhost:8001/ready   |
| Prometheus metrics | http://localhost:8001/metrics |
| MinIO API          | http://localhost:9000         |
| MinIO console      | http://localhost:9001         |

Stop services:

```bash
docker compose down
```

Remove local volumes:

```bash
docker compose down -v
```

## Demo Accounts

The backend seeds demo data automatically when the user table is empty.

| Role          | Employee ID | Password   | Main permissions                                                     |
| ------------- | ----------- | ---------- | -------------------------------------------------------------------- |
| Event manager | `MGR001`    | `password` | Create events, publish/close events, upload assets, check in tickets |
| Employee      | `EMP001`    | `password` | Browse events, apply for tickets, view/cancel tickets                |
| Employee      | `EMP002`    | `password` | Browse events, apply for tickets, view/cancel tickets                |
| HR            | `HR001`     | `password` | View reports and export CSV files                                    |

## Local Development

### Start infrastructure only

```bash
docker compose up -d postgres redis minio
```

If the backend is run directly on the host instead of inside Docker, update `.env` so host-based services are reachable:

```env
MINIO_ENDPOINT=localhost:9000
MINIO_PUBLIC_ENDPOINT=http://localhost:9000
REDIS_URL=redis://localhost:6379
```

### Backend

```bash
cd backend
go mod download
go run main.go
```

The backend listens on `http://localhost:8001` by default and loads `.env` from the backend directory or the repository root.

### Frontend

```bash
cd frontend
npm ci
npm run dev
```

The Vite dev server listens on `http://localhost:5173` and proxies `/v1` requests to `http://localhost:8001`.

## Main API Areas

All application APIs are under `/v1` except `/health`, `/ready`, and `/metrics`.

| Area             | Example endpoints                                                                                     | Access                                |
| ---------------- | ----------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Auth             | `POST /v1/auth/login`, `GET /v1/auth/me`                                                              | Login / authenticated users           |
| Events           | `GET /v1/events`, `GET /v1/events/:id`                                                                | Authenticated users                   |
| Event management | `POST /v1/events`, `PUT /v1/events/:id`, `PATCH /v1/events/:id/publish`, `PATCH /v1/events/:id/close` | Event manager                         |
| Applications     | `POST /v1/applications`, `GET /v1/applications/my`, `POST /v1/applications/:id/cancel`                | Employee                              |
| Queue status     | `GET /v1/applications/queue/:idempotency_key`                                                         | Employee                              |
| Tickets          | `GET /v1/tickets/my`, `POST /v1/tickets/:id/cancel`                                                   | Employee                              |
| Check-in         | `POST /v1/checkin`, `GET /v1/checkins`                                                                | Event manager                         |
| Upload           | `POST /v1/upload`                                                                                     | Event manager                         |
| Reports          | `GET /v1/reports/overview`, `GET /v1/reports/events/:id/stats`, `GET /v1/reports/events/:id/export`   | HR / event manager depending on route |

The OpenAPI contract is available at `api/openapi.yaml`.

## Implementation Notes

### Authentication and RBAC

The backend issues JWTs after employee ID/password login. Auth middleware validates the token, stores user identity and role in the Gin context, and role middleware protects employee, event manager, and HR routes.

### Event lifecycle

Events include `publish_time`, `start_time`, `apply_deadline`, and `end_time`. The service validates this order:

```text
publish_time <= start_time
publish_time <= apply_deadline
start_time <= end_time
apply_deadline <= end_time
```

A scheduler periodically publishes due drafts, closes events after the application deadline, and marks finished events as ended. Redis is used as a scheduler lock when multiple backend replicas are running.

### Ticket application and anti-oversell

The default booking path is synchronous. The employee submits `event_id`, `ticket_type_id`, `quantity`, and an `idempotency_key`. Redis is used as a fast inventory counter and guard, while PostgreSQL performs the final transactional update. The ticket type row is updated only when enough inventory remains, and each successful application immediately creates approved tickets.

The service also checks per-event user limits and uses the idempotency key to avoid duplicate inventory deduction when the same request is retried.

### Queue-based booking mode

For high-concurrency traffic, enable Redis Stream waiting-room mode:

```env
TICKET_QUEUE_ENABLED=true
TICKET_QUEUE_STREAM=ticket:applications
TICKET_QUEUE_GROUP=ticket-workers
TICKET_QUEUE_WORKERS=8
TICKET_QUEUE_MAX_WAITING=100000
TICKET_QUEUE_RESERVATION_TTL_SECONDS=900
TICKET_QUEUE_STATUS_TTL_SECONDS=3600
```

In this mode, `POST /v1/applications` can return `202 Accepted` with status `queued`. Worker goroutines consume the Redis Stream and write approved applications/tickets to PostgreSQL. Employees can poll queue state through:

```http
GET /v1/applications/queue/{idempotency_key}
```

More details are in `docs/queue_waiting_room.md` and `load-test/README.md`.

### Dynamic QR-code check-in

Employee tickets have a base `qr_token`. The frontend generates a dynamic payload:

```text
qr_token|otp
```

The OTP is generated from the ticket token and a 60-second time window. The backend accepts the current, previous, and next windows to tolerate small clock differences. A successful check-in marks the ticket as used and creates a `Checkin` record.

### Redis caching

Redis is used for inventory counters, event list/detail cache, employee application/ticket short-lived cache, queue status, and distributed locks. Event read-cache TTL is controlled by:

```env
EVENT_LIST_CACHE_TTL_SECONDS=5
```

Set it to `0` or leave it empty to disable event read caching.

### File upload and storage

Event managers can upload PNG, JPG/JPEG, and PDF files through `POST /v1/upload`. The backend validates file extension, content type, maximum size, and image dimensions before uploading to MinIO. The MinIO service creates the configured bucket if it does not already exist.

### Observability

The backend exposes Prometheus metrics at `/metrics`, including:

* `http_requests_total`
* `http_request_duration_seconds`
* `ticket_requests_total`
* `ticket_request_duration_seconds`
* `ticket_remaining_count`
* `ticket_verified_total`

Kubernetes monitoring manifests are under `k8s/monitoring/`.

## Testing

### Backend

```bash
docker compose up -d postgres redis minio
cd backend
go test ./...
```

Race detection and coverage:

```bash
cd backend
go test -race -coverprofile=coverage.txt -covermode=atomic ./...
go tool cover -func=coverage.txt
```

### Frontend

```bash
cd frontend
npm ci
npm run test
npm run test:coverage
npm run lint
npm run build
```

### Frontend E2E

```bash
cd frontend
npm ci
npx playwright install --with-deps
npm run test:e2e
```

### CI-style full stack

```bash
docker compose -f docker-compose.ci.yml up -d --build
```

## Load Testing

The `load-test/` directory contains k6 scripts for booking, read-heavy browsing, mixed journeys, smoke tests, and Kubernetes-based runs.

Small local check:

```bash
docker compose up -d --build
chmod +x load-test/run_load_test.sh
TOTAL_USERS=10 TOTAL_QUOTA=10 VUS=10 ITERATIONS=1 KEEP_DATA=1 ./load-test/run_load_test.sh
```

Results are written to:

```text
load-test/results/<RUN_ID>/
load-test/results/<RUN_ID>.zip
```

## Kubernetes Deployment

The `k8s/` directory contains manifests for frontend, backend, PostgreSQL, Redis, MinIO, ConfigMap, Secret, Ingress, TLS-related resources, and monitoring.

For local Kubernetes testing:

```bash
kubectl apply -f k8s/ -R
kubectl get pods -w
```

The frontend service is configured as a local-friendly NodePort:

```text
http://localhost:30080
```

The deployment workflow builds changed frontend/backend images, pushes them to GHCR with the commit SHA tag, applies the Kubernetes manifests, updates changed deployments, and waits for rollout completion.

## CI/CD and Code Quality

GitHub Actions includes:

* Trivy filesystem security scan
* Backend tests with PostgreSQL and Redis services
* Backend coverage report generation
* golangci-lint
* Frontend Vitest coverage
* Frontend lint and production build
* Docker Compose based integration/E2E test job
* Playwright E2E artifact upload on failure

`sonar-project.properties` is included for SonarCloud analysis using backend and frontend coverage reports.

## Environment Variables

Most local settings are defined in `.env.example`.

| Variable                                                                                               | Purpose                                    |
| ------------------------------------------------------------------------------------------------------ | ------------------------------------------ |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT`                                                         | PostgreSQL configuration                   |
| `DATABASE_URL`                                                                                         | Optional full PostgreSQL connection string |
| `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME_MINUTES`                               | Backend database connection pool           |
| `REDIS_URL`, `REDIS_PORT`                                                                              | Redis configuration                        |
| `JWT_SECRET`                                                                                           | JWT signing secret                         |
| `PORT`                                                                                                 | Backend port                               |
| `ALLOWED_ORIGINS`                                                                                      | CORS allow list                            |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET_NAME`, `MINIO_PUBLIC_ENDPOINT` | MinIO storage configuration                |
| `EVENT_LIST_CACHE_TTL_SECONDS`                                                                         | Event list/detail Redis cache TTL          |
| `TICKET_QUEUE_ENABLED` and related `TICKET_QUEUE_*` variables                                          | Redis Stream waiting-room mode             |
| `TEST_USER_PASSWORD`                                                                                   | Password used by tests where applicable    |

For normal local Docker Compose usage, copying `.env.example` to `.env` is enough.
