{ pkgs, lib, ... }:

{
  # Runtime
  virtualisation.docker = {
    enable = true;
    autoPrune.enable = true;
  };
  virtualisation.oci-containers.backend = "docker";

  # Containers
  virtualisation.oci-containers.containers."jellyseerr" = {
    image = "docker.io/fallenbagel/jellyseerr:latest";
    environmentFiles = [
      "testdata/input.env"
    ];
    volumes = [
      "/var/volumes/jellyseerr:/app/config:rw"
      "books:/books:rw"
    ];
    cmd = [ "ls" "-la" "/" ];
    labels = {
      "traefik.enable" = "true";
      "traefik.http.routers.jellyseerr.middlewares" = "chain-authelia@file";
      "traefik.http.routers.jellyseerr.rule" = "Host(`requests.hello.us`)";
      "traefik.http.routers.jellyseerr.tls.certresolver" = "htpc";
    };
    dependsOn = [
      "myproject-sabnzbd"
    ];
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--cpu-quota=1.5"
      "--cpus=1"
      "--dns=1.1.1.1"
      "--health-cmd='curl -f http://localhost/\${POTATO}'"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--memory-reservation=524288000b"
      "--memory=1048576000b"
      "--network=container:myproject-sabnzbd"
    ];
  };
  systemd.services."docker-jellyseerr" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "on-failure";
      RestartMaxDelaySec = lib.mkOverride 500 "1m";
      RestartSec = lib.mkOverride 500 "100ms";
      RestartSteps = lib.mkOverride 500 9;
    };
    startLimitBurst = 3;
    unitConfig = {
      StartLimitIntervalSec = lib.mkOverride 500 120;
    };
    after = [
      "docker-volume-books.service"
    ];
    requires = [
      "docker-volume-books.service"
    ];
    partOf = [
      "docker-compose-myproject-root.target"
    ];
    unitConfig.UpheldBy = [
      "docker-myproject-sabnzbd.service"
    ];
    wantedBy = [
      "docker-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."myproject-sabnzbd" = {
    image = "lscr.io/linuxserver/sabnzbd";
    environmentFiles = [
      "testdata/input.env"
    ];
    volumes = [
      "/var/volumes/sabnzbd:/config:rw"
      "storage:/storage:rw"
    ];
    labels = {
      "traefik.enable" = "true";
      "traefik.http.routers.sabnzbd.middlewares" = "chain-authelia@file";
      "traefik.http.routers.sabnzbd.rule" = "Host(`hey.hello.us`) && PathPrefix(`/sabnzbd`)";
      "traefik.http.routers.sabnzbd.tls.certresolver" = "htpc";
    };
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--health-cmd='curl -f http://localhost/'"
      "--hostname=sabnzbd"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network-alias=sabnzbd"
      "--network=myproject_default"
    ];
  };
  systemd.services."docker-myproject-sabnzbd" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "always";
      RestartMaxDelaySec = lib.mkOverride 500 "1m";
      RestartSec = lib.mkOverride 500 "100ms";
      RestartSteps = lib.mkOverride 500 9;
      RuntimeMaxSec = lib.mkOverride 500 10;
    };
    unitConfig = {
      Description = lib.mkOverride 500 "This is the sabnzbd container!";
    };
    after = [
      "docker-network-myproject_default.service"
      "docker-volume-storage.service"
    ];
    requires = [
      "docker-network-myproject_default.service"
      "docker-volume-storage.service"
    ];
    partOf = [
      "docker-compose-myproject-root.target"
    ];
    wantedBy = [
      "docker-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."photoprism-mariadb" = {
    image = "docker.io/library/mariadb:10.9";
    environmentFiles = [
      "testdata/input.env"
    ];
    volumes = [
      "/var/volumes/photoprism-mariadb:/var/lib/mysql:rw"
      "photos:/photos:rw"
    ];
    user = "1000:1000";
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--health-cmd='[\"curl\",\"-f\",\"http://localhost\"]'"
      "--health-interval=1m30s"
      "--health-retries=3"
      "--health-start-period=40s"
      "--health-timeout=10s"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network=host"
    ];
  };
  systemd.services."docker-photoprism-mariadb" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "always";
      RestartMaxDelaySec = lib.mkOverride 500 "1m";
      RestartSec = lib.mkOverride 500 "100ms";
      RestartSteps = lib.mkOverride 500 9;
    };
    startLimitBurst = 10;
    unitConfig = {
      StartLimitIntervalSec = lib.mkOverride 500 "infinity";
    };
    after = [
      "docker-volume-photos.service"
    ];
    requires = [
      "docker-volume-photos.service"
    ];
    partOf = [
      "docker-compose-myproject-root.target"
    ];
    wantedBy = [
      "docker-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."torrent-client" = {
    image = "docker.io/haugene/transmission-openvpn";
    environmentFiles = [
      "testdata/input.env"
    ];
    volumes = [
      "/etc/localtime:/etc/localtime:ro"
      "/var/volumes/transmission/config:/config:rw"
      "/var/volumes/transmission/scripts:/scripts:rw"
      "storage:/storage:rw"
    ];
    ports = [
      "9091:9091/tcp"
    ];
    labels = {
      "autoheal" = "true";
      "traefik.enable" = "true";
      "traefik.http.routers.transmission.middlewares" = "chain-authelia@file";
      "traefik.http.routers.transmission.rule" = "Host(`hey.hello.us`) && PathPrefix(`/transmission`)";
      "traefik.http.routers.transmission.tls.certresolver" = "htpc";
      "traefik.http.services.transmission.loadbalancer.server.port" = "9091";
    };
    dependsOn = [
      "myproject-sabnzbd"
    ];
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--add-host=abc:93.184.216.34"
      "--add-host=abc:::1"
      "--cap-add=NET_ADMIN"
      "--device=/dev/net/tun:/dev/net/tun"
      "--dns=8.8.4.4"
      "--dns=8.8.8.8"
      "--network-alias=my-torrent-client"
      "--network-alias=transmission"
      "--network=myproject_something"
      "--no-healthcheck"
      "--privileged"
      "--shm-size=67108864"
      "--sysctl=net.ipv6.conf.all.disable_ipv6=0"
    ];
  };
  systemd.services."docker-torrent-client" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "on-failure";
    };
    startLimitBurst = 3;
    unitConfig = {
      StartLimitIntervalSec = lib.mkOverride 500 "infinity";
    };
    after = [
      "docker-network-myproject_something.service"
      "docker-volume-storage.service"
    ];
    requires = [
      "docker-network-myproject_something.service"
      "docker-volume-storage.service"
    ];
    partOf = [
      "docker-compose-myproject-root.target"
    ];
    unitConfig.UpheldBy = [
      "docker-myproject-sabnzbd.service"
    ];
    wantedBy = [
      "docker-compose-myproject-root.target"
    ];
  };
  virtualisation.oci-containers.containers."traefik" = {
    image = "docker.io/library/traefik";
    environmentFiles = [
      "testdata/input.env"
    ];
    volumes = [
      "/var/run/podman/podman.sock:/var/run/docker.sock:ro"
      "/var/volumes/traefik:/etc/traefik:rw"
    ];
    ports = [
      "80:80/tcp"
      "443:443/tcp"
    ];
    labels = {
      "traefik.enable" = "true";
      "traefik.http.routers.traefik.entrypoints" = "https";
      "traefik.http.routers.traefik.middlewares" = "chain-authelia@file";
      "traefik.http.routers.traefik.rule" = "Host(`hey.hello.us`) && (PathPrefix(`/api`) || PathPrefix(`/dashboard`))";
      "traefik.http.routers.traefik.service" = "api@internal";
      "traefik.http.routers.traefik.tls.certresolver" = "htpc";
    };
    dependsOn = [
      "sabnzbd"
    ];
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network=container:sabnzbd"
      "--runtime=nvidia"
      "--security-opt=label=disable"
    ];
  };
  systemd.services."docker-traefik" = {
    serviceConfig = {
      Restart = lib.mkOverride 500 "no";
      RestartMaxDelaySec = lib.mkOverride 500 "1m";
      RestartSec = lib.mkOverride 500 "100ms";
      RestartSteps = lib.mkOverride 500 9;
    };
    unitConfig = {
      AllowIsolate = lib.mkOverride 500 true;
    };
    partOf = [
      "docker-compose-myproject-root.target"
    ];
    unitConfig.UpheldBy = [
      "docker-sabnzbd.service"
    ];
    wantedBy = [
      "docker-compose-myproject-root.target"
    ];
  };

  # Networks
  systemd.services."docker-network-myproject_default" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.docker}/bin/docker network rm -f myproject_default";
    };
    script = ''
      docker network inspect myproject_default || docker network create myproject_default
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-network-myproject_something" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.docker}/bin/docker network rm -f myproject_something";
    };
    script = ''
      docker network inspect myproject_something || docker network create myproject_something --label=test-label=okay
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };

  # Volumes
  systemd.services."docker-volume-books" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect books || docker volume create books --opt=device=/mnt/media/Books --opt=o=bind --opt=type=none
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-volume-photos" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect photos || docker volume create photos --opt=device=/mnt/photos --opt=o=bind --opt=type=none --label=test-label=okay
    '';
    partOf = [ "docker-compose-myproject-root.target" ];
    wantedBy = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-volume-storage" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect storage || docker volume create storage --opt=device=/mnt/media --opt=o=bind --opt=type=none
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
