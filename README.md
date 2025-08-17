# compose2nix module

NixOS module that uses compose2nix to convert docker compose files to nix

## Module

### Installation (Flakes)

Add `compose2nix.nixosModules.compose-containers` module to your system

```
{
  inputs.compose2nix.url = "github:aksiksi/compose2nix/549403c432e5058f6800e6208f5a7d1f071f1338";

  outputs = { self, nixpkgs, compose2nix }: {
    nixosConfigurations."yourhostname" = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        compose2nix.nixosModules.compose-containers
        ./configuration.nix
      ];
    };
  };
}
```

### Usage

Use module by specifying `virtualisation.oci-containers.compose-containers.<name>`

Specify the location of the docker compose file in the `path` attribute

You can also specify options for the compose2nix command in the `convertOptions` attribute

An example:

```
{ pkgs, config, ... }:
{
  virtualisation.oci-containers.compose-containers."myservice" = {
    path = ./myservice/docker-compose.yml;
    convertOptions = {
      default_stop_timeout = "5m";
    };
  };
}
```
