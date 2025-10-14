build:
	mkdir -p bin/ && go build -o bin/ .

test:
	go test -v

coverage:
	go test -v -covermode=count -coverprofile=coverage.out

flake:
	nix build -L .#packages.x86_64-linux.default

# Updates nixpkgs, Go module deps, and runs tests.
update-deps:
	go get -u github.com/Masterminds/sprig/v3 github.com/compose-spec/compose-go/v2 github.com/joho/godotenv
	go mod tidy
	nix flake update nixpkgs
	make flake
	make shell
	make test

# Pulls in all build dependencies into a shell.
shell:
	nix develop -c zsh

# This brings up two NixOS VMs - one for Docker and one for Podman - and ensures that
# the compose2nix generated config works when loaded into NixOS.
nixos-test:
	./nixos-test/update.sh
	nix build .#checks.x86_64-linux.integrationTest

nixos-test-verbose:
	./nixos-test/update.sh
	nix build -L .#checks.x86_64-linux.integrationTest

.PHONY: build coverage flake nixos-test nixos-test-verbose test
