{ ... }:

{
  name = "basic";
  nodes = {
    docker = { pkgs, ... }: {
      imports = [
        ./docker-compose.nix
      ];
      virtualisation.graphics = false;
      system.stateVersion = "23.05";
    };
    podman = { pkgs, ... }: {
      imports = [
        ./podman-compose.nix
      ];
      virtualisation.graphics = false;
      system.stateVersion = "23.05";
    };
  };
  skipLint = true;
  # TODO(aksiksi): This currently takes way too long to pull the images.
  # Perhaps we need to build and use local images?
  testScript = ''
    import time

    def num_running_containers() -> int:
      stdout = m.execute(f"{runtime} ps --format '{{.ID}}: {{.Names}} - {{.State}}' | grep running | wc -l")[1]
      return int(stdout.strip())

    start_all()
    d = {"docker": docker, "podman": podman}

    for runtime, m in d.items():
      # Create required directories for Docker Compose volumes and bind mounts.
      m.execute("mkdir -p /mnt/media/Books")
      m.execute("mkdir -p /var/volumes/alpine")

      # Wait for root Compose service to come up.
      m.wait_for_unit(f"{runtime}-compose-myproject-root.target")

      # Wait for container services.
      m.wait_for_unit(f"{runtime}-myproject-alpine.service")

      # Poll container state.
      num_attempts = 0
      while num_attempts < 20:
        if num_running_containers() == 1:
          break
        num_attempts += 1
        time.sleep(3)
      else:
        raise Exception("timeout!")
  '';
}
