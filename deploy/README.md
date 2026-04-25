# AI Orchestrator V5 — Deployment

## Quick Start

```bash
# 1. Configure environment
cp deploy/.env.example deploy/.env
# Edit .env with your values

# 2. Start services
docker compose -f deploy/docker-compose.yml up -d

# 3. Check health
curl http://localhost:8080/health
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| Orchestrator | 8080 | HTTP API |
| PostgreSQL | 5432 | State storage |
| Redis | 6379 | Queue/Cache |
| Prometheus | 9090 | Metrics (optional) |
| Grafana | 3000 | Dashboards (optional) |

## Configuration

### Required Variables (.env)

```bash
# Generate secure API key:
openssl rand -base64 32

# Update API_KEY in .env
# Update POSTGRES_PASSWORD in .env
```

### Optional: Enable Monitoring

```bash
docker compose -f deploy/docker-compose.yml --profile monitoring up -d prometheus grafana
```

## Commands

```bash
# Build images
make build

# Start all services
make up

# Stop all services
make down

# View logs
make logs

# Check health
make health

# Clean up (removes volumes!)
make clean
```

## API Usage

```bash
# Health check
curl http://localhost:8080/health

# Create task (with API key)
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR-API-KEY" \
  -d '{"goal": "Fix failing test"}'
```

## Troubleshooting

### Services won't start

```bash
# Check logs
docker compose -f deploy/docker-compose.yml logs

# Check ports
netstat -tlnp | grep -E '8080|5432|6379'
```

### Database connection issues

```bash
# Connect to database
make db-console

# Check migrations
docker exec -i deploy-postgres-1 psql -U admin -d orchestrator -c "\dt"
```

### Out of memory

```bash
# Check docker stats
docker stats
```

## Production Considerations

1. **Security**: Change default API key and passwords in `.env`
2. **SSL/TLS**: Put behind nginx/caddy reverse proxy
3. **Backups**: Use `make db-backup`
4. **Monitoring**: Enable Prometheus + Grafana
5. **Health Checks**: Already configured in docker-compose.yml

## Dockerfile Multi-stage Build

The Dockerfile builds both orchestrator and worker binaries using Go 1.26:

```dockerfile
# Build orchestrator
docker build -f deploy/Dockerfile --target orchestrator -t orchestrator .

# Build worker  
docker build -f deploy/Dockerfile --target worker -t worker .
```

## Scaling Workers

Add more workers in docker-compose.yml:

```yaml
  worker-3:
    image: orchestrator-worker:latest
    environment:
      - WORKER_ID=worker-3
```