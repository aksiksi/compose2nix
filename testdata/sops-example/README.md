# sops-nix Integration Example

This example demonstrates how to use compose2nix with sops-nix for managing secrets.

## Prerequisites

1. Your NixOS configuration must already have sops-nix configured with the required secrets
2. The secrets referenced in compose labels must already be defined in your sops configuration

## Usage

```bash
compose2nix \
  --sops_file ./secrets/pinnacle.yaml \
  --inputs compose.yml \
  --output docker-compose.nix
```

## Compose Labels

Use the `compose2nix.sops.secret` label with comma-separated secret names to reference sops secrets:

```yaml
services:
  webapp:
    image: nginx:latest
    labels:
      - "compose2nix.sops.secret=example.env,folder.example-2.env"
```

## Generated NixOS Configuration

The secrets will be added to the container's `environmentFiles` array:

```nix
virtualisation.oci-containers.containers."webapp" = {
  image = "nginx:latest";
  
  environmentFiles = [
    config.sops.secrets."example.env".path
    config.sops.secrets."folder.example-2.env".path
  ];
};
```

## Important Notes

- The `--sops_file` flag automatically handles environment files for sops secrets
- No sops configuration is generated - it assumes sops is already configured in your NixOS config
- Referenced secrets must exist in your sops configuration, or validation will fail
- Secrets are made available as environment files, not individual environment variables