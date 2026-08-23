.PHONY: test api
test:
	go test ./internal/dominio/...
api:
	go run ./cmd/api
