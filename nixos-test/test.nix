{ pkgs, ... }:

let
  # Use pre-pulled images to avoid pulling having to pull images
  # inside the test VMs.
  # https://nixos.org/manual/nixpkgs/stable/#ssec-pkgs-dockerTools-fetchFromRegistry
  sabnzbdImage = pkgs.dockerTools.pullImage {
    imageName = "lscr.io/linuxserver/sabnzbd";
    finalImageTag = "latest";
    imageDigest = "sha256:10de547c287d318200b776ccd2adb7e826a0040e70287877b92a9f9ceccd6840";
    sha256 = "sha256-jeOo0b1SvCK1cQ9lxq3Pt1CWVRtu6RqiFw/tdzWjEkc=";
  };
  radarrImage = pkgs.dockerTools.pullImage {
    imageName = "lscr.io/linuxserver/radarr";
    finalImageTag = "develop";
    imageDigest = "sha256:7e5682d40b8ff3276df8b71cc420bd9c714dbc9e22b2f4110055999c75802452";
    sha256 = "sha256-GcZZFMjSBOjdMDm9sZ1AGt8pPNIXLwxXRERUjFgNNHs=";
  };
in
{
  name = "basic";
  nodes = {
    docker = { pkgs, ... }: {
      imports = [
        ./docker-compose.nix
      ];
      # Container DNS.
      networking.firewall.allowedUDPPorts = [ 53 ];
      virtualisation.graphics = false;
      virtualisation.oci-containers.containers."radarr".imageFile = radarrImage;
      virtualisation.oci-containers.containers."myproject-sabnzbd".imageFile = sabnzbdImage;
      system.stateVersion = "23.05";
    };
    podman = { pkgs, ... }: {
      imports = [
        ./podman-compose.nix
      ];
      # Container DNS.
      networking.firewall.allowedUDPPorts = [ 53 ];
      virtualisation.graphics = false;
      virtualisation.oci-containers.containers."radarr".imageFile = radarrImage;
      virtualisation.oci-containers.containers."myproject-sabnzbd".imageFile = sabnzbdImage;
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
      m.succeed("mkdir -p /var/volumes/radarr")
      m.succeed("mkdir -p /var/volumes/sabnzbd")

    for runtime, m in d.items():
      # Wait for root Compose service to come up.
      m.wait_for_unit(f"{runtime}-compose-myproject-root.target")

      # Wait for container services.
      m.wait_for_unit(f"{runtime}-radarr.service")
      m.wait_for_unit(f"{runtime}-myproject-sabnzbd.service")
  '';
}
