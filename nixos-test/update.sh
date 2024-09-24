#!/usr/bin/env bash

export TIMEZONE="America/New_York"

# Generate NixOS configs for each runtime.
make build
bin/compose2nix \
  -runtime=docker \
  -inputs=nixos-test/docker-compose.yml \
  -output=nixos-test/docker-compose.nix \
  -check_systemd_mounts \
  -include_env_files=true \
  -use_upheld_by
bin/compose2nix \
  -runtime=podman \
  -inputs=nixos-test/docker-compose.yml \
  -output=nixos-test/podman-compose.nix \
  -check_systemd_mounts \
  -use_upheld_by \
  -include_env_files=true
