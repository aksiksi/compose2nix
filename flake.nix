{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: {
    nixosModules.compose-containers = { pkgs, config, lib, ... } @ args: let
      inputs = { inherit nixpkgs; compose2nix = self; };
      args' = args // { inherit inputs; };
      module = import ./modules/compose-containers.nix;
    in module args';
  };
}
