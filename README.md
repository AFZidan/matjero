# Distributed Commerce Platform

Engineering foundation for a multilingual, multi-market distributed commerce platform.

## Phase 0

Phase 0 establishes architecture documentation, Go backend app foundations, frontend app foundations, PostgreSQL migrations, local infrastructure, and CI. Commerce business features begin in later phases.

## Local Development

```sh
cp .env.example .env
docker compose up -d postgres redis rabbitmq zitadel
go test ./...
npm install
npm run lint
npm run typecheck
npm run test
docker compose config --quiet
```

Kafka is intentionally not part of the Phase 0 runtime.
