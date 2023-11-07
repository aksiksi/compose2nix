# References:
# https://blog.lenny.ninja/part-1-quickly-packaging-services-using-nix-flakes.html
# https://ayats.org/blog/no-flake-utils/
# https://ryantm.github.io/nixpkgs/builders/trivial-builders/#trivial-builder-runCommand
# https://nixpkgs-manual-sphinx-markedown-example.netlify.app/development/writing-modules.xml.html#structure-of-nixos-modules
{
  description = "minimal configuration for nix-compose";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: let
    supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
    forAllSystems = function: nixpkgs.lib.genAttrs supportedSystems (system: function nixpkgs.legacyPackages.${system});
    pname = "nix-compose";
    version = "v0.0.1";
  in {
    # Nix package
    packages = forAllSystems (pkgs: {
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
        cfg = config.nix-compose;
      in {
        options.nix-compose = {
          # https://nixos.org/manual/nixos/stable/#sec-option-declarations
          enable = mkEnableOption "nix-compose";
          paths = mkOption {
            type = types.listOf types.pathInStore;
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
            description = lib.mdDoc "Environment variables. These will be merged with environment files, if any.";
          };
          envFiles = mkOption {
            type = types.listOf types.path;
            default = [];
            description = lib.mdDoc "One or more paths to environment files.";
          };
          envFilesOnly = mkOption {
            type = types.bool;
            default = false;
            description = lib.mdDoc "Only include env files in the output Nix file.";
          };
          autoStart = mkOption {
            type = types.bool;
            default = true;
            description = lib.mdDoc "Auto-start all containers.";
          };
          serviceInclude = mkOption {
            type = types.bool;
            default = "";
            description = lib.mdDoc "Regex pattern for Docker Compose services to include.";
          };
        };
        configs = mkIf cfg.enable {
          nix-compose = {
            output = pkgs.runCommand "run-nix-compose" {
              buildInputs = [ pkgs.nix-compose ];
              env = cfg.env;
            } ''
              ${pkgs.nix-compose}/bin/nix-compose \
                -paths='${concatStringsSep "," cfg.paths}' \
                -runtime=${cfg.runtime} \
                -project=${cfg.project} \
                -project_separator='${cfg.projectSeparator}' \
                -env_files='${concatStringsSep "," cfg.envFiles}' \
                -env_files_only=${cfg.envFilesOnly} \
                -auto_start=${cfg.autoStart} \
                -service_include='${cfg.serviceInclude}' \
                -output=$out/output.nix
            '';
          };
        };
      };
    });
  };
}
