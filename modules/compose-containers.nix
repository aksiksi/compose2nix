{ pkgs, config, lib, inputs, ... } @ args:
with lib; let
  importCompose = { name, path, convertOptions ? {} }: let
    system = pkgs.stdenv.buildPlatform.system;
    pkgs' = with inputs; import nixpkgs { inherit system; };
    options = convertOptions // {
      runtime = config.virtualisation.oci-containers.backend;
      write_nix_setup = false;
      project = name;
      inputs = path;
    };
    asValue = v: with builtins; if isBool v then toJSON v
      else if isInt v || isFloat v then toString v else v;
    asArg = { name, value }: "-${name}=${asValue value}";
    asArgs = options: map asArg (attrsToList options);
    drv = derivation {
      name = "${name}-compose.nix";
      inherit system;
      builder = "${pkgs'.bash}/bin/bash";
      args = [ "-c" ''
        exec ${pkgs'.compose2nix}/bin/compose2nix "$@" -output="$out"
      '' "_" ] ++ (asArgs options);
    };
  in import drv;
  cfg = config.virtualisation.oci-containers;
  mkModule = { name, value }: (importCompose ({ inherit name; } // value));
  modules = map mkModule (attrsToList cfg.compose-containers);
  configs = map (m: m args) modules;
  passConfigs = paths: let
    singlePass = l: setAttrByPath l (mkMerge (map (c: attrByPath l {} c) configs));
    lists = map (strings.splitString ".") paths;
  in mergeAttrsList (map singlePass lists);
in {
  options.virtualisation.oci-containers.compose-containers = mkOption {
    default = { };
    type = types.attrsOf (types.submodule ({ name, ... }: {
      options = {
        path = mkOption {
          type = with types; path;
          description = "Path to compose file.";
        };
        convertOptions = mkOption {
          type = with types; attrsOf (oneOf [ str int float bool ]);
          default = { };
          example = {
            default_stop_timeout = "1m";
          };
          description = "Options for {command}`compose2nix`.";
        };
      };
    }));
    description = "OCI (Docker) compose containers to run as systemd services.";
  };

  config = passConfigs [
    "virtualisation.oci-containers.containers"
    "systemd"
  ];
}
