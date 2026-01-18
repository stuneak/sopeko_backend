# Sopeko Backend

Stock mention aggregator that scrapes Reddit (r/pennystocks, r/investing, r/stocks, r/wallstreetbets) for ticker mentions, tracks prices via Yahoo Finance, and provides an API to see how mentioned stocks performed over time.

## Tech Stack

Go 1.25, Gin, PostgreSQL 16, sqlc, gocron, Docker, Nginx

## Local Development

### Prerequisites

- Go 1.25+
- Docker & Docker Compose

### Setup

1. Create `.env` file:
```bash
DB_DRIVER=postgres
POSTGRES_USER=postgres
POSTGRES_PASSWORD=password
POSTGRES_DB=sopeko
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
DB_SOURCE=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable
APP_PORT=8080
SERVER_ADDRESS=0.0.0.0:${APP_PORT}
GIN_MODE=debug
```

2. Run with Docker (includes hot reload):
```bash
make uplocal
```

3. Stop:
```bash
make downlocal
```

### Other Commands

```bash
make server       # Run without Docker (needs DB running)
make migrateup    # Run migrations
make migratedown  # Rollback migration
make sqlc         # Generate DB code
make test         # Run tests
```

## API

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /mentions/{username}` | Returns user's first mention per ticker with price change since mention |

## Production

### Requirements

- VPS with Docker & Docker Compose
- Domain with DNS pointing to VPS
- Ports 80/443 open

### GitHub Secrets

```
POSTGRES_USER
POSTGRES_PASSWORD
POSTGRES_DB
GITHUB_REPOSITORY
VPS_HOST
VPS_USER
VPS_PASSWORD
VPS_PORT
```

### Deployment

Push to `main` branch triggers GitHub Actions:
1. Builds Docker image → pushes to ghcr.io
2. SSHs to VPS → runs `docker compose -f docker-compose.prod.yml up -d`
3. Nginx handles SSL via Let's Encrypt (auto-renews)

### Architecture

```
Internet → Nginx (SSL/443) → Go App (8080) → PostgreSQL (5432)
```

## Scheduled Jobs

| Job | Schedule | Purpose |
|-----|----------|---------|
| NASDAQ sync | Daily | Sync ticker list |
| Price fetch | 10:00 AM ET | Update all prices |
| Reddit scrape | Every 4h (staggered) | Scrape each subreddit |
