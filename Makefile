build:
	mkdir -p bin/ && go build -o bin/ ./cmd/nixose

.PHONY: build run
