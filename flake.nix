# References:
# https://blog.lenny.ninja/part-1-quickly-packaging-services-using-nix-flakes.html
# https://ayats.org/blog/no-flake-utils/
# https://nixos.org/manual/nixos/stable/#sec-writing-modules
# https://nixos.org/manual/nixpkgs/stable/#ssec-language-go
{
  description = "minimal configuration for compose2nix";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: let
    supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
    forAllSystems = function: nixpkgs.lib.genAttrs supportedSystems (system: function nixpkgs.legacyPackages.${system});
    pname = "compose2nix";
    version = "0.0.1";
  in {
    # Nix package
    packages = forAllSystems (pkgs: {
      # TODO(aksiksi): Pull from GitHub.
      default = pkgs.buildGoModule {
        inherit pname;
        inherit version;
        src = ./.;
        vendorSha256 = "sha256-+Ht+cdaTwmfdZ2lMnSuX9EBkU2V8rXyH1qBet8pZ+dU=";
      };
    });

    # Development shell
    devShells = forAllSystems (pkgs: {
      default = pkgs.mkShell {
        buildInputs = [ pkgs.go pkgs.gopls ];
      };
    });

    # NixOS module
    nixosModules = forAllSystems (pkgs: {
      default = { config, lib, pkgs, ... }:
      with lib; let
        cfg = config.compose2nix;
      in {
        options.compose2nix = {
          # https://nixos.org/manual/nixos/stable/#sec-option-declarations
          enable = mkOption {
            type = types.bool;
            default = false;
            description = lib.mdDoc "Enable compose2nixos.";
          };
          paths = mkOption {
            type = types.listOf types.path;
            description = lib.mdDoc "One or more paths to Docker Compose files.";
          };
          runtime = mkOption {
            type = types.enum [ "docker" "podman" ];
            default = "podman";
            description = lib.mdDoc "Container runtime to use.";
          };
          project = mkOption {
            type = types.str;
            default = "";
            description = lib.mdDoc "Project name. Used as a prefix for all generated resources.";
          };
          projectSeparator = mkOption {
            type = types.str;
            default = "_";
            description = lib.mdDoc "Defines the prefix between the project and resource name - i.e., [project][sep][resource].";
          };
          env = mkOption {
            type = types.attrsOf types.str;
            default = {};
            description = lib.mdDoc "Environment to use. Note that these will be merged with environment files, if any.";
          };
          envFiles = mkOption {
            type = types.listOf types.path;
            default = [];
            description = lib.mdDoc "One or more paths to environment files.";
          };
          envFilesOnly = mkOption {
            type = types.bool;
            default = false;
            description = lib.mdDoc "Only include env files in the output Nix file. Useful in cases where env variables contain secrets.";
          };
          serviceInclude = mkOption {
            type = types.str;
            default = "";
            description = lib.mdDoc "Regex pattern for Docker Compose services to include.";
          };
          autoStart = mkOption {
            type = types.bool;
            default = true;
            description = lib.mdDoc "Auto-start all containers.";
          };
          output = mkOption {
            type = types.anything;
            description = lib.mdDoc "Output config.";
          };
        };
        config = {
          # runCommandLocal ensures that we always build this derivation on the local machine.
          # This allows us to circumvent the Nix binary cache and minimize the time spent outside
          # of building the derivation.
          # https://nixos.org/manual/nixpkgs/stable/#trivial-builder-runCommandLocal
          compose2nix.output = mkIf (cfg.enable) (import pkgs.runCommandLocal "compose2nix" {
            env = cfg.env;
            buildInputs = [ pkgs.compose2nix ];
          } ''
            ${pkgs.compose2nix}/bin/compose2nix \
              -paths='${concatStringsSep "," cfg.paths}' \
              -runtime=${cfg.runtime} \
              -project=${cfg.project} \
              -project_separator='${cfg.projectSeparator}' \
              -env_files='${concatStringsSep "," cfg.envFiles}' \
              -env_files_only=${cfg.envFilesOnly} \
              -service_include='${cfg.serviceInclude}' \
              -auto_start=${cfg.autoStart} \
              -output=$out
          '');
        };
      };
    });
  };
}
