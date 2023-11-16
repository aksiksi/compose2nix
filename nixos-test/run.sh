#!/bin/bash
./nixos-test/update.sh
nix build -L .#checks.x86_64-linux.integrationTest --option sandbox false
