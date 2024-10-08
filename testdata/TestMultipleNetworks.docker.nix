{ pkgs, lib, ... }:

{
  # Runtime
  virtualisation.docker = {
    enable = true;
    autoPrune.enable = true;
  };
  virtualisation.oci-containers.backend = "docker";

  # Containers
  virtualisation.oci-containers.containers."traefik" = {
    image = "docker.io/library/traefik";
    ports = [
      "80:80/tcp"
      "443:443/tcp"
    ];
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--network-alias=my-container"
      "--network-alias=traefik"
      "--network=myproject_test1"
      "--network=myproject_test2"
      "--network=myproject_test3"
    ];
  };
  systemd.services."docker-traefik" = {
    serviceConfig = {
      Restart = lib.mkOverride 90 "always";
      RestartMaxDelaySec = lib.mkOverride 90 "1m";
      RestartSec = lib.mkOverride 90 "100ms";
      RestartSteps = lib.mkOverride 90 9;
    };
    after = [
      "docker-network-myproject_test1.service"
      "docker-network-myproject_test2.service"
      "docker-network-myproject_test3.service"
    ];
    requires = [
      "docker-network-myproject_test1.service"
      "docker-network-myproject_test2.service"
      "docker-network-myproject_test3.service"
    ];
  };

  # Networks
  systemd.services."docker-network-myproject_test1" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "docker network rm -f myproject_test1";
    };
    script = ''
      docker network inspect myproject_test1 || docker network create myproject_test1 --internal
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-network-myproject_test2" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "docker network rm -f myproject_test2";
    };
    script = ''
      docker network inspect myproject_test2 || docker network create myproject_test2
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-network-myproject_test3" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "docker network rm -f myproject_test3";
    };
    script = ''
      docker network inspect myproject_test3 || docker network create myproject_test3
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };

  # Root service
  # When started, this will automatically create all resources and start
  # the containers. When stopped, this will teardown all resources.
  systemd.targets."docker-compose-myproject-root" = {
    unitConfig = {
      Description = "Root target generated by compose2nix.";
    };
  };
}
