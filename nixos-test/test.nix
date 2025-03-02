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
        ];
        # Override restart value and ensure it takes effect.
        systemd.services."podman-service-b" = {
          serviceConfig = {
            Restart = lib.mkForce "on-success";
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
  '';
}
