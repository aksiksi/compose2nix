#!/bin/bash

export TIMEZONE="America/New_York"

# Generate NixOS configs for each runtime.
make build
bin/compose2nix \
  -inputs=nixos-test/docker-compose.yml \
  -output=nixos-test/docker-compose.nix \
  -runtime=docker
bin/compose2nix \
  -inputs=nixos-test/docker-compose.yml \
  -output=nixos-test/podman-compose.nix \
  -runtime=podman
