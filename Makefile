GO ?= go
ARGS ?=
GORUNFLAGS ?= -trimpath

.PHONY: up down logs run seed compare-index clean-cache

up:
	docker-compose up -d

down:
	docker-compose down -v

logs:
	docker compose logs -f mysql

run:
	$(GO) run $(GORUNFLAGS) ./cmd/slowlab $(ARGS)

seed:
	$(GO) run $(GORUNFLAGS) ./cmd/slowlab -skip-scenarios $(ARGS)

compare-index:
	$(GO) run $(GORUNFLAGS) ./cmd/slowlab -skip-seed -orders 0 $(ARGS)

clean-cache:
	@echo "nothing to clean"
