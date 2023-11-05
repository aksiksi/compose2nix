build:
	mkdir -p bin/ && go build -o bin/ ./cmd/nixose

test:
	go test

.PHONY: build test
