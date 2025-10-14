# sops integration test

## To update secrets

```sh
cd nixos-test/sops
nix-shell -p sops --run "SOPS_AGE_KEY_FILE=age-key.txt sops edit secrets.yaml"
```
