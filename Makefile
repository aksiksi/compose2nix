build:
	mkdir -p bin/ && go build -o bin/ ./cmd/compose2nix

test:
	go test -v

.PHONY: build test
