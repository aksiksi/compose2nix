{ pkgs, lib, ... }:

{
  # Runtime
  virtualisation.podman = {
    enable = true;
    autoPrune.enable = true;
    dockerCompat = true;
    defaultNetwork.settings = {
      # Required for container networking to be able to use names.
      dns_enabled = true;
    };
  };
  virtualisation.oci-containers.backend = "podman";

  # Containers
  virtualisation.oci-containers.containers."test-service" = {
    image = "nginx:latest";
    volumes = [
      "test_my-volume:/mnt/volume:rw"
    ];
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--network-alias=service"
      "--network=test_my-network"
    ];
  };
  systemd.services."podman-test-service" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "no";
    };
    after = [
      "podman-network-test_my-network.service"
      "podman-volume-test_my-volume.service"
    ];
    requires = [
      "podman-network-test_my-network.service"
      "podman-volume-test_my-volume.service"
    ];
  };

  # Networks
  systemd.services."podman-network-test_my-network" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "podman network rm -f test_my-network";
    };
    script = ''
      podman network inspect test_my-network || podman network create test_my-network --driver=bridge
    '';
  };

  # Volumes
  systemd.services."podman-volume-test_my-volume" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      podman volume inspect test_my-volume || podman volume create test_my-volume
    '';
  };
}
