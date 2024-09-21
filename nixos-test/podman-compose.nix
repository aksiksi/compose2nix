# Auto-generated using compose2nix v0.2.3-pre.
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
  virtualisation.oci-containers.containers."myproject-no-restart" = {
    image = "docker.io/library/nginx:stable-alpine-slim";
    log-driver = "journald";
    extraOptions = [
      "--network-alias=no-restart"
      "--network=myproject_default"
    ];
  };
  systemd.services."podman-myproject-no-restart" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "no";
    };
    after = [
      "podman-network-myproject_default.service"
    ];
    requires = [
      "podman-network-myproject_default.service"
    ];
    partOf = [
      "podman-compose-myproject-root.target"
    ];
    wantedBy = [
      "podman-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."myproject-service-a" = {
    image = "docker.io/library/nginx:stable-alpine-slim";
    environment = {
      "TZ" = "America/New_York";
      "test.key" = "ABC";
    };
    volumes = [
      "/var/volumes/service-a:/config:rw"
      "storage:/storage:rw"
    ];
    labels = {
      "compose2nix.systemd.service.Restart" = "no";
      "compose2nix.systemd.service.RuntimeMaxSec" = "360";
      "compose2nix.systemd.unit.Description" = "This is the service-a container!";
    };
    log-driver = "journald";
    extraOptions = [
      "--cpus=0.5"
      "--network-alias=service-a"
      "--network=myproject_default"
    ];
  };
  systemd.services."podman-myproject-service-a" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "no";
      RuntimeMaxSec = lib.mkOverride 500 360;
    };
    unitConfig = {
      Description = lib.mkOverride 500 "This is the service-a container!";
    };
    after = [
      "podman-network-myproject_default.service"
      "podman-volume-storage.service"
    ];
    requires = [
      "podman-network-myproject_default.service"
      "podman-volume-storage.service"
    ];
    partOf = [
      "podman-compose-myproject-root.target"
    ];
    wantedBy = [
      "podman-compose-myproject-root.target"
    ];
    unitConfig.RequiresMountsFor = [
      "/var/volumes/service-a"
    ];
  };
  virtualisation.oci-containers.containers."service-b" = {
    image = "docker.io/library/nginx:stable-alpine-slim";
    environment = {
      "TZ" = "America/New_York";
    };
    volumes = [
      "/var/volumes/service-b:/config:rw"
      "myproject_books:/books:rw"
      "storage:/storage:rw"
    ];
    labels = {
      "compose2nix.systemd.service.RuntimeMaxSec" = "360";
      "compose2nix.systemd.unit.AllowIsolate" = "no";
    };
    dependsOn = [
      "myproject-service-a"
    ];
    log-driver = "journald";
    extraOptions = [
      "--health-cmd=echo abc && true"
      "--ip=192.168.8.20"
      "--network-alias=service-b"
      "--network=myproject_something"
    ];
  };
  systemd.services."podman-service-b" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "on-failure";
      RuntimeMaxSec = lib.mkOverride 500 360;
    };
    startLimitBurst = 3;
    unitConfig = {
      AllowIsolate = lib.mkOverride 500 "no";
      StartLimitIntervalSec = lib.mkOverride 500 "infinity";
    };
    after = [
      "podman-network-myproject_something.service"
      "podman-volume-myproject_books.service"
      "podman-volume-storage.service"
    ];
    requires = [
      "podman-network-myproject_something.service"
      "podman-volume-myproject_books.service"
      "podman-volume-storage.service"
    ];
    partOf = [
      "podman-compose-myproject-root.target"
    ];
    upheldBy = [
      "podman-myproject-service-a.service"
    ];
    wantedBy = [
      "podman-compose-myproject-root.target"
    ];
    unitConfig.RequiresMountsFor = [
      "/var/volumes/service-b"
    ];
  };

  # Networks
  systemd.services."podman-network-myproject_default" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "podman network rm -f myproject_default";
    };
    script = ''
      podman network inspect myproject_default || podman network create myproject_default
    '';
    partOf = [ "podman-compose-myproject-root.target" ];
    wantedBy = [ "podman-compose-myproject-root.target" ];
  };
  systemd.services."podman-network-myproject_something" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "podman network rm -f myproject_something";
    };
    script = ''
      podman network inspect myproject_something || podman network create myproject_something --subnet=192.168.8.0/24 --gateway=192.168.8.1 --label=test-label=okay
    '';
    partOf = [ "podman-compose-myproject-root.target" ];
    wantedBy = [ "podman-compose-myproject-root.target" ];
  };

  # Volumes
  systemd.services."podman-volume-myproject_books" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    unitConfig.RequiresMountsFor = [
      "/mnt/media/Books"
    ];
    script = ''
      podman volume inspect myproject_books || podman volume create myproject_books --opt=device=/mnt/media/Books --opt=o=bind --opt=type=none
    '';
    partOf = [ "podman-compose-myproject-root.target" ];
    wantedBy = [ "podman-compose-myproject-root.target" ];
  };
  systemd.services."podman-volume-storage" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    unitConfig.RequiresMountsFor = [
      "/mnt/media"
    ];
    script = ''
      podman volume inspect storage || podman volume create storage --opt=device=/mnt/media --opt=o=bind --opt=type=none
    '';
    partOf = [ "podman-compose-myproject-root.target" ];
    wantedBy = [ "podman-compose-myproject-root.target" ];
  };

  # Root service
  # When started, this will automatically create all resources and start
  # the containers. When stopped, this will teardown all resources.
  systemd.targets."podman-compose-myproject-root" = {
    unitConfig = {
      Description = "Root target generated by compose2nix.";
    };
    wantedBy = [ "multi-user.target" ];
  };
}
