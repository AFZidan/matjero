.PHONY: test go-test docker-config docker-build migrate-check

test: go-test docker-config migrate-check

go-test:
	go test ./...

docker-config:
	docker compose config --quiet

docker-build:
	docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/workers/general-worker -t commerce-general-worker:foundation .
	docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/workers/scheduler -t commerce-scheduler:foundation .

migrate-check:
	test -f migrations/000001_event_delivery_foundation.up.sql
	test -f migrations/000001_event_delivery_foundation.down.sql
