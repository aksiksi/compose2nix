{ pkgs, lib, ... }:

{
  # Runtime
  virtualisation.docker = {
    enable = true;
    autoPrune.enable = true;
  };
  virtualisation.oci-containers.backend = "docker";

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
  systemd.services."docker-test-service" = {
    serviceConfig = {
      Restart = lib.mkOverride 90 "no";
    };
    after = [
      "docker-network-test_my-network.service"
      "docker-volume-test_my-volume.service"
    ];
    requires = [
      "docker-network-test_my-network.service"
      "docker-volume-test_my-volume.service"
    ];
  };

  # Networks
  systemd.services."docker-network-test_my-network" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "docker network rm -f test_my-network";
    };
    script = ''
      docker network inspect test_my-network || docker network create test_my-network --driver=bridge
    '';
  };

  # Volumes
  systemd.services."docker-volume-test_my-volume" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect test_my-volume || docker volume create test_my-volume
    '';
  };
}
