build:
	mkdir -p bin/ && go build -o bin/ ./cmd/compose2nixos

.PHONY: build run
