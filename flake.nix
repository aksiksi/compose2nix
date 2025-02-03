{
  description = "A tool to automatically generate a NixOS config from a Docker Compose project.";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    onchg.url = "github:aksiksi/onchg-rs";
    onchg.inputs.nixpkgs.follows = "nixpkgs";

    alejandra.url = "github:kamadorueda/alejandra/3.1.0";
    alejandra.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = { self, nixpkgs, onchg, alejandra, ... }: let
    supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
    forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    pkgsFor = system: nixpkgs.legacyPackages.${system};
    pname = "compose2nix";
    owner = "aksiksi";
    # LINT.OnChange(version)
    version = "0.3.2-pre";
    # LINT.ThenChange(main.go:version)
  in {
    # Formatter to be used by helped func

    # Nix package
    packages = forAllSystems (system:
      let pkgs = pkgsFor system; in {
        default = pkgs.buildGoModule {
          inherit pname;
          inherit version;
          src = ./.;
          vendorHash = "sha256-fqJTZPDaWJB05nyLPif968Ek2pJQzkKpr6lrdgSY9MM=";
        };
      }
    );

    # Development shell
    devShells = forAllSystems (system:
      let pkgs = pkgsFor system; in {
        default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.gopls pkgs.nixfmt-rfc-style alejandra.defaultPackage.${system} ];
          # Add a Git pre-commit hook.
          shellHook = onchg.shellHook.${system};
        };
        ci = pkgs.mkShell {
          # We already have Go installed.
          buildInputs = [ pkgs.nixfmt-rfc-style ];
        };
      }
    );

    # Run:
    # nix build .#checks.x86_64-linux.integrationTest
    # To run interactively:
    # nix build .#checks.x86_64-linux.integrationTest.driverInteractive
    # See: https://nixos.org/manual/nixos/stable/index.html#sec-running-nixos-tests-interactively
    checks.x86_64-linux = let
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in {
      # This test is meant to be run by nixos-test/test.sh.
      # https://nixos.org/manual/nixos/stable/index.html#sec-nixos-tests
      # https://nix.dev/tutorials/nixos/integration-testing-using-virtual-machines
      integrationTest = pkgs.nixosTest (import ./nixos-test/test.nix);
    };
  };
}
