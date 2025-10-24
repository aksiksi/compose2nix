{ pkgs, ... }:

let
  # Use pre-pulled image to avoid having to pull images inside the test VMs.
  # https://nixos.org/manual/nixpkgs/stable/#ssec-pkgs-dockerTools-fetchFromRegistry
  nginxImage = pkgs.dockerTools.pullImage {
    imageName = "docker.io/library/nginx";
    finalImageTag = "stable-alpine-slim";
    imageDigest = "sha256:f5fb3bd2fc68f768b81bccad0161f8100ac52b2de4d7b6128421edd2ce136296";
    sha256 = "sha256-yRDW3G/JA4WjVOul4zCHE/Xnpk+7qPGtkueiFje6EOE=";
  };
  sops-nix = builtins.fetchTarball {
    url = "https://github.com/Mic92/sops-nix/archive/5a7d18b5c55642df5c432aadb757140edfeb70b3.tar.gz";
    sha256 = "1dk0kjms9a6in9flaz4pxrlngxj235xivgwcy9bvw60yy3brxvbr";
  };
  common = {
    virtualisation.graphics = false;
    virtualisation.oci-containers.containers."myproject-service-a".imageFile = nginxImage;
    virtualisation.oci-containers.containers."service-b".imageFile = nginxImage;
    virtualisation.oci-containers.containers."myproject-no-restart".imageFile = nginxImage;
    environment.systemPackages = [ pkgs.jq ];
    system.stateVersion = "23.05";
  };
in
{
  name = "basic";
  nodes = {
    docker =
      { pkgs, lib, ... }:
      {
        imports = [
          ./docker-compose.nix
        ];

        custom.prefix.myproject.enable = true;

        # Override restart value and ensure it takes effect.
        systemd.services."docker-service-b" = {
          serviceConfig = {
            Restart = lib.mkForce "on-success";
          };
        };
      }
      // common;
    podman =
      { pkgs, lib, ... }:
      {
        imports = [
          ./podman-compose.nix
          ./sops/podman-compose.nix
          "${sops-nix}/modules/sops"
        ];

        # Override restart value and ensure it takes effect.
        systemd.services."podman-service-b" = {
          serviceConfig = {
            Restart = lib.mkForce "on-success";
          };
        };

        virtualisation.oci-containers.containers."sopstest-app".imageFile = nginxImage;

        # Copy age key early in boot to ensure the sops-secrets service will
        # have access to it for decryption.
        boot.initrd.postDeviceCommands = ''
          cp ${./sops/age-key.txt} /run/age-key.txt
          chmod 700 /run/age-key.txt
        '';

        sops = {
          age.keyFile = "/run/age-key.txt";
          defaultSopsFile = ./sops/secrets.yaml;
          secrets = {
            "api.env" = {};
            "env/env-1.env" = {};
          };
        };
      }
      // common;
  };
  # https://nixos.org/manual/nixos/stable/index.html#sec-nixos-tests
  testScript = ''
    d = {"docker": docker, "podman": podman}

    start_all()

    # Create required directories for Docker Compose volumes and bind mounts.
    for runtime, m in d.items():
      m.succeed("mkdir -p /mnt/media")
      m.succeed("mkdir -p /mnt/media/Books")
      m.succeed("mkdir -p /var/volumes/service-a")
      m.succeed("mkdir -p /var/volumes/service-b")

      # Create env file used by service-a.
      m.succeed("echo 'ABC=100' > /tmp/test.env")

    for runtime, m in d.items():
      # Wait for root Compose service to come up.
      m.wait_for_unit(f"{runtime}-compose-myproject-root.target")

      # Wait for container services.
      m.wait_for_unit(f"{runtime}-myproject-service-a.service")
      m.wait_for_unit(f"{runtime}-service-b.service")
      m.wait_for_unit(f"{runtime}-myproject-no-restart.service")

      # Wait until the health check succeeds.
      m.wait_until_succeeds(f"{runtime} inspect service-b | jq .[0].State.Health.Status | grep healthy", timeout=30)

      # Ensure that services have correct systemd restart settings.
      m.succeed(f"systemctl show -p Restart {runtime}-myproject-service-a.service | grep -E '=no$'")
      m.succeed(f"systemctl show -p Restart {runtime}-service-b.service | grep -E '=on-success$'")
      m.succeed(f"systemctl show -p Restart {runtime}-myproject-no-restart.service | grep -E '=no$'")

      # Ensure we can reach a container in the same network. Regression test
      # for DNS settings, especially for Podman.
      m.succeed(f"{runtime} exec -it myproject-service-a wget http://no-restart")

      # Verify UpheldBy behavior by stopping the volume service and ensuring
      # that the container goes down, then comes up after the volume is started.
      m.systemctl(f"stop {runtime}-volume-storage.service")
      m.wait_until_fails(f"{runtime}-myproject-service-a.service")
      m.systemctl(f"start {runtime}-volume-storage.service")
      m.wait_for_unit(f"{runtime}-myproject-service-a.service")

      # Stop the root unit.
      m.systemctl(f"stop {runtime}-compose-myproject-root.target")

    # Test sops integration, only for Podman.
    print("Testing sops integration...")

    # Wait for sops compose services to come up.
    podman.wait_for_unit(f"{runtime}-compose-sopstest-root.target")
    podman.wait_for_unit(f"{runtime}-sopstest-app.service")

    # Ensure that secret env files have been loaded by the container.
    m.succeed(f"{runtime} exec -it sopstest-app env | grep 'VERSION' | grep '10'")
    m.succeed(f"{runtime} exec -it sopstest-app env | grep 'APP' | grep 'myapp'")

    # Stop the sops root unit.
    podman.systemctl(f"stop {runtime}-compose-sopstest-root.target")
  '';
}

