{ pkgs, ... }:

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
    environment = {
      PGID = "1000";
      PUID = "1000";
      TZ = "America/New_York";
    };
    volumes = [
      "/var/volumes/jellyseerr:/app/config:rw"
      "books:/books:rw"
    ];
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
      "--dns=1.1.1.1"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network-alias=jellyseerr"
      "--network=container:myproject-sabnzbd"
    ];
  };
  systemd.services."docker-jellyseerr" = {
    serviceConfig = {
      Restart = "on-failure";
      RestartSec = "5s";
    };
    after = [
      "mnt-media.mount"
    ];
    requires = [
      "mnt-media.mount"
    ];
    startLimitBurst = 3;
    startLimitIntervalSec = 120;
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  virtualisation.oci-containers.containers."myproject-sabnzbd" = {
    image = "lscr.io/linuxserver/sabnzbd";
    environment = {
      DOCKER_MODS = "ghcr.io/gilbn/theme.park:sabnzbd";
      PGID = "1000";
      PUID = "1000";
      TP_DOMAIN = "hey.hello.us\/themepark";
      TP_HOTIO = "false";
      TP_THEME = "potato";
      TZ = "America/New_York";
    };
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
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network-alias=sabnzbd"
      "--network=myproject-default"
    ];
  };
  systemd.services."docker-myproject-sabnzbd" = {
    serviceConfig = {
      Restart = "always";
      RuntimeMaxSec = 10;
    };
    unitConfig = {
      Description = "This is the sabnzbd container!";
    };
    after = [
      "mnt-media.mount"
    ];
    requires = [
      "mnt-media.mount"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  virtualisation.oci-containers.containers."photoprism-mariadb" = {
    image = "docker.io/library/mariadb:10.9";
    environment = {
      MARIADB_AUTO_UPGRADE = "1";
      MARIADB_DATABASE = "photoprism";
      MARIADB_INITDB_SKIP_TZINFO = "1";
      MARIADB_PASSWORD = "insecure";
      MARIADB_ROOT_PASSWORD = "insecure";
      MARIADB_USER = "photoprism";
    };
    volumes = [
      "/var/volumes/photoprism-mariadb:/var/lib/mysql:rw"
      "photos:/photos:rw"
    ];
    user = "1000:1000";
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network-alias=photoprism-mariadb"
      "--network=host"
    ];
  };
  systemd.services."docker-photoprism-mariadb" = {
    serviceConfig = {
      Restart = "always";
      RestartSec = "3m0s";
    };
    after = [
      "mnt-photos.mount"
    ];
    requires = [
      "mnt-photos.mount"
    ];
    startLimitBurst = 10;
    startLimitIntervalSec = 86400;
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  virtualisation.oci-containers.containers."torrent-client" = {
    image = "docker.io/haugene/transmission-openvpn";
    environment = {
      GLOBAL_APPLY_PERMISSIONS = "false";
      LOCAL_NETWORK = "192.168.0.0/16";
      PGID = "1000";
      PUID = "1000";
      TRANSMISSION_DHT_ENABLED = "false";
      TRANSMISSION_DOWNLOAD_DIR = "/storage/Downloads/transmission";
      TRANSMISSION_HOME = "/config/transmission-home";
      TRANSMISSION_INCOMPLETE_DIR = "/storage/Downloads/transmission/incomplete";
      TRANSMISSION_INCOMPLETE_DIR_ENABLED = "true";
      TRANSMISSION_PEX_ENABLED = "false";
      TRANSMISSION_SCRIPT_TORRENT_DONE_ENABLED = "true";
      TRANSMISSION_SCRIPT_TORRENT_DONE_FILENAME = "/config/transmission-unpack.sh";
      TZ = "America/New_York";
    };
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
      "--cap-add=NET_ADMIN"
      "--device=/dev/net/tun:/dev/net/tun"
      "--dns=8.8.4.4"
      "--dns=8.8.8.8"
      "--network-alias=my-torrent-client"
      "--network-alias=transmission"
      "--network=myproject-something"
      "--privileged"
    ];
  };
  systemd.services."docker-torrent-client" = {
    serviceConfig = {
      Restart = "on-failure";
    };
    after = [
      "mnt-media.mount"
    ];
    requires = [
      "mnt-media.mount"
    ];
    startLimitBurst = 3;
    startLimitIntervalSec = 86400;
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  virtualisation.oci-containers.containers."traefik" = {
    image = "docker.io/library/traefik";
    environment = {
      CLOUDFLARE_API_KEY = "yomama";
      CLOUDFLARE_EMAIL = "aaa@aaa.com";
    };
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
      "--network-alias=traefik"
      "--network=container:sabnzbd"
    ];
  };
  systemd.services."docker-traefik" = {
    serviceConfig = {
      Restart = "none";
    };
    partOf = [ "docker-compose-myproject-root.target" ];
  };

  # Networks
  systemd.services."docker-network-myproject-default" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.docker}/bin/docker network rm -f myproject-default";
    };
    script = ''
      docker network inspect myproject-default || docker network create myproject-default
    '';
    before = [
      "docker-myproject-sabnzbd.service"
    ];
    requiredBy = [
      "docker-myproject-sabnzbd.service"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-network-myproject-something" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.docker}/bin/docker network rm -f myproject-something";
    };
    script = ''
      docker network inspect myproject-something || docker network create myproject-something --label=test-label=okay
    '';
    before = [
      "docker-torrent-client.service"
    ];
    requiredBy = [
      "docker-torrent-client.service"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };

  # Volumes
  systemd.services."docker-volume-books" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect books || docker volume create books --opt device=/mnt/media/Books,o=bind,type=none
    '';
    before = [
      "docker-jellyseerr.service"
    ];
    requiredBy = [
      "docker-jellyseerr.service"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-volume-photos" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect photos || docker volume create photos --opt device=/mnt/photos,o=bind,type=none --label=test-label=okay
    '';
    before = [
      "docker-photoprism-mariadb.service"
    ];
    requiredBy = [
      "docker-photoprism-mariadb.service"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };
  systemd.services."docker-volume-storage" = {
    path = [ pkgs.docker ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
    };
    script = ''
      docker volume inspect storage || docker volume create storage --opt device=/mnt/media,o=bind,type=none
    '';
    before = [
      "docker-myproject-sabnzbd.service"
      "docker-torrent-client.service"
    ];
    requiredBy = [
      "docker-myproject-sabnzbd.service"
      "docker-torrent-client.service"
    ];
    partOf = [ "docker-compose-myproject-root.target" ];
  };

  # Root service
  # When started, this will automatically create all resources and start
  # the containers. When stopped, this will teardown all resources.
  systemd.targets."docker-compose-myproject-root" = {
    unitConfig = {
      Description = "Root target generated by compose2nix.";
    };
    wants = [
      "docker-jellyseerr.service"
      "docker-myproject-sabnzbd.service"
      "docker-photoprism-mariadb.service"
      "docker-torrent-client.service"
      "docker-traefik.service"
      "docker-network-myproject-default.service"
      "docker-network-myproject-something.service"
      "docker-volume-books.service"
      "docker-volume-photos.service"
      "docker-volume-storage.service"
    ];
  };
}
