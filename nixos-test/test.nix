{ pkgs, ... }:

let
  # Use pre-pulled images.
  sabnzbdImage = pkgs.dockerTools.pullImage {
    imageName = "lscr.io/linuxserver/sabnzbd";
    finalImageTag = "latest";
    imageDigest = "sha256:10de547c287d318200b776ccd2adb7e826a0040e70287877b92a9f9ceccd6840";
    sha256 = "sha256-jeOo0b1SvCK1cQ9lxq3Pt1CWVRtu6RqiFw/tdzWjEkc=";
  };
in
{
  name = "basic";
  nodes = {
    docker = { pkgs, ... }: {
      imports = [
        ./docker-compose.nix
      ];
      virtualisation.oci-containers.containers."myproject-sabnzbd".imageFile = sabnzbdImage;
      virtualisation.graphics = false;
      system.stateVersion = "23.05";
    };
    podman = { pkgs, ... }: {
      imports = [
        ./podman-compose.nix
      ];
      virtualisation.oci-containers.containers."myproject-sabnzbd".imageFile = sabnzbdImage;
      virtualisation.graphics = false;
      system.stateVersion = "23.05";
    };
  };
  testScript = ''
    d = {"docker": docker, "podman": podman}

    start_all()

    # Create required directories for Docker Compose volumes and bind mounts.
    for runtime, m in d.items():
      m.succeed("mkdir -p /mnt/media")
      m.succeed("mkdir -p /mnt/media/Books")
      m.succeed("mkdir -p /var/volumes/sabnzbd")

    for runtime, m in d.items():
      # Wait for root Compose service to come up.
      m.wait_for_unit(f"{runtime}-compose-myproject-root.target")

      # Wait for container services.
      m.wait_for_unit(f"{runtime}-myproject-sabnzbd.service")
  '';
}
