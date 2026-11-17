.PHONY: check test vet fmt build run docker parity

# The gate: formatting, vet and unit tests.
check: fmt vet test

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -o bin/celle ./cmd/celle

# Runs with .env loaded (CELLE_TOKENS is required).
run:
	set -a && . ./.env && set +a && go run ./cmd/celle

docker:
	docker build -t celle:dev .

# Compare celle with Banko's /api/utils. Needs a Banko checkout with vendor/
# installed: make parity BANKO=/path/to/banko. See qa/banko-parity/README.md.
parity:
	@test -n "$(BANKO)" || (echo "set BANKO=/path/to/banko"; exit 1)
	go run ./qa/banko-parity gen > /tmp/celle-parity-cases.jsonl
	cd "$(BANKO)" && php $(CURDIR)/qa/banko-parity/banko-harness.php < /tmp/celle-parity-cases.jsonl > /tmp/celle-parity-banko.jsonl
	go run ./qa/banko-parity compare /tmp/celle-parity-cases.jsonl /tmp/celle-parity-banko.jsonl
