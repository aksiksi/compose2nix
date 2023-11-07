build:
	mkdir -p bin/ && go build -o bin/ ./cmd/nix-compose

test:
	go test -v

.PHONY: build test
