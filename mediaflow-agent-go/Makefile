.PHONY: run test fmt docker-up docker-down

run:
	go run ./cmd/mediaflow-agent

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

docker-up:
	docker compose up --build

docker-down:
	docker compose down
