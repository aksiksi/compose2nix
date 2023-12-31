# Auto-generated using compose2nix v0.1.6.
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
  virtualisation.oci-containers.containers."myproject-sabnzbd" = {
    image = "lscr.io/linuxserver/sabnzbd:latest";
    environment = {
      TZ = "America/New_York";
    };
    volumes = [
      "/mnt/media:/storage:rw"
      "/var/volumes/sabnzbd:/config:rw"
    ];
    log-driver = "journald";
    extraOptions = [
      "--network-alias=sabnzbd"
      "--network=myproject-default"
    ];
  };
  systemd.services."podman-myproject-sabnzbd" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "always";
      RuntimeMaxSec = lib.mkOverride 500 360;
    };
    unitConfig = {
      Description = lib.mkOverride 500 "This is the sabnzbd container!";
    };
    after = [
      "podman-network-myproject-default.service"
    ];
    requires = [
      "podman-network-myproject-default.service"
    ];
    partOf = [
      "podman-compose-myproject-root.target"
    ];
    wantedBy = [
      "podman-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."radarr" = {
    image = "lscr.io/linuxserver/radarr:develop";
    environment = {
      TZ = "America/New_York";
    };
    volumes = [
      "/mnt/media:/storage:rw"
      "/var/volumes/radarr:/config:rw"
    ];
    dependsOn = [
      "myproject-sabnzbd"
    ];
    log-driver = "journald";
    extraOptions = [
      "--network-alias=radarr"
      "--network=myproject-default"
    ];
  };
  systemd.services."podman-radarr" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "always";
    };
    after = [
      "podman-network-myproject-default.service"
    ];
    requires = [
      "podman-network-myproject-default.service"
    ];
    partOf = [
      "podman-compose-myproject-root.target"
    ];
    unitConfig.UpheldBy = [
      "podman-myproject-sabnzbd.service"
    ];
    wantedBy = [
      "podman-compose-myproject-root.target"
    ];
  };

  # Networks
  systemd.services."podman-network-myproject-default" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.podman}/bin/podman network rm -f myproject-default";
    };
    script = ''
      podman network inspect myproject-default || podman network create myproject-default --opt isolate=true
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
