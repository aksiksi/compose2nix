{
  inputs = {};

  outputs = { ... }: {
    nixosModules.compose-containers = import ./modules/compose-containers.nix;
  };
}
