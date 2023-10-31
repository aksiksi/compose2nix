build:
	mkdir -p bin/ && go build -o bin/compose-to-nixos .

.PHONY: build run
