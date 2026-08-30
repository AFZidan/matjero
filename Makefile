.PHONY: test go-test frontend-install frontend-lint frontend-typecheck frontend-test docker-config docker-build migrate-check

test: go-test frontend-lint frontend-typecheck frontend-test docker-config migrate-check

go-test:
	go test ./...

frontend-install:
	npm install

frontend-lint:
	npm run lint

frontend-typecheck:
	npm run typecheck

frontend-test:
	npm run test

docker-config:
	docker compose config --quiet

docker-build:
	docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/admin-api -t commerce-admin-api:foundation .
	docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/workers/general-worker -t commerce-general-worker:foundation .
	docker build -f docker/web-app.Dockerfile --build-arg WORKSPACE=@commerce/admin-web -t commerce-admin-web:foundation .
	docker build -f docker/web-app.Dockerfile --build-arg WORKSPACE=@commerce/storefront-web -t commerce-storefront-web:foundation .

migrate-check:
	test -f migrations/000001_event_delivery_foundation.up.sql
	test -f migrations/000001_event_delivery_foundation.down.sql
