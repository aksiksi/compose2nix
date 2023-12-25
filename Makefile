build:
	mkdir -p bin/ && go build -o bin/ .

test:
	go test -v

flake:
	nix build -L .#packages.x86_64-linux.default

# This brings up two NixOS VMs - one for Docker and one for Podman - and ensures that
# the compose2nix generated config works when loaded into NixOS.
nixos-test:
	./nixos-test/update.sh
	nix build -L .#checks.x86_64-linux.integrationTest

.PHONY: build flake nixos-test test
