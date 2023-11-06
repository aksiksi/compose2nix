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
    ];
    labels = {
      "traefik.enable" = "true";
      "traefik.http.routers.jellyseerr.middlewares" = "chain-authelia@file";
      "traefik.http.routers.jellyseerr.rule" = "Host(`requests.hello.us`)";
      "traefik.http.routers.jellyseerr.tls.certresolver" = "htpc";
    };
    dependsOn = [
      "myproject_sabnzbd"
    ];
    logDriver = "journald";
    autoStart = false;
    extraOptions = [
      "--network-alias=jellyseerr"
      "--dns=1.1.1.1"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
      "--network=container:myproject_sabnzbd"
    ];
  };
  systemd.services."docker-jellyseerr" = {
    serviceConfig = {
      Restart = "always";
    };
  };
  virtualisation.oci-containers.containers."myproject_sabnzbd" = {
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
    logDriver = "journald";
    autoStart = false;
    extraOptions = [
      "--network=myproject_default"
      "--network-alias=sabnzbd"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
  };
  systemd.services."docker-myproject_sabnzbd" = {
    serviceConfig = {
      Restart = "always";
      RuntimeMaxSec = 10;
    };
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
    ];
    user = "1000:1000";
    logDriver = "journald";
    autoStart = false;
    extraOptions = [
      "--network=myproject_default"
      "--network-alias=photoprism-mariadb"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
  };
  systemd.services."docker-photoprism-mariadb" = {
    serviceConfig = {
      Restart = "always";
    };
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
      "myproject_sabnzbd"
    ];
    logDriver = "journald";
    autoStart = false;
    extraOptions = [
      "--network=myproject_default"
      "--network-alias=transmission"
      "--dns=8.8.8.8"
      "--dns=8.8.4.4"
      "--privileged"
      "--cap-add=NET_ADMIN"
      "--device=/dev/net/tun:/dev/net/tun"
    ];
  };
  systemd.services."docker-torrent-client" = {
    serviceConfig = {
      Restart = "on-failure";
    };
    startLimitBurst = 3;
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
    logDriver = "journald";
    autoStart = false;
    extraOptions = [
      "--network=myproject_default"
      "--network-alias=traefik"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
  };
  systemd.services."docker-traefik" = {
    serviceConfig = {
      Restart = "always";
    };
  };

  # Networks
  systemd.services."create-docker-network-myproject_default" = {
    serviceConfig.Type = "oneshot";
    path = [ pkgs.docker ];
    script = ''
      docker network inspect myproject_default || docker network create myproject_default
    '';
    wantedBy = [
      "docker-myproject_sabnzbd.service"
      "docker-photoprism-mariadb.service"
      "docker-torrent-client.service"
      "docker-traefik.service"
    ];
  };

  # Volumes
  systemd.services."create-docker-volume-books" = {
    serviceConfig.Type = "oneshot";
    path = [ pkgs.docker ];
    script = ''
      docker volume inspect books || docker volume create books --opt device=/mnt/media/Books,o=bind,type=none
    '';
  };
  systemd.services."create-docker-volume-photos" = {
    serviceConfig.Type = "oneshot";
    path = [ pkgs.docker ];
    script = ''
      docker volume inspect photos || docker volume create photos --opt device=/mnt/photos,o=bind,type=none
    '';
  };
  systemd.services."create-docker-volume-storage" = {
    serviceConfig.Type = "oneshot";
    path = [ pkgs.docker ];
    script = ''
      docker volume inspect storage || docker volume create storage --opt device=/mnt/media,o=bind,type=none
    '';
    wantedBy = [
      "docker-myproject_sabnzbd.service"
      "docker-torrent-client.service"
    ];
  };

  # Scripts
  up = writeShellScript "compose-myproject_up.sh" ''
    echo "TODO: Create resources."
  '';
  down = writeShellScript "compose-myproject_down.sh" ''
    echo "TODO: Remove resources."
  '';
}
