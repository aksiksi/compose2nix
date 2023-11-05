{ pkgs, ... }:

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
    extraOptions = [
      "--network=default"
      "--network-alias=jellyseerr"
      "--dns=1.1.1.1"
      "--log-driver=json-file"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
    autoStart = false;
  };
  systemd.services."podman-jellyseerr" = {
    serviceConfig = {
      Restart = "always";
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
    extraOptions = [
      "--network=default"
      "--network-alias=photoprism-mariadb"
      "--log-driver=json-file"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
    user = "1000:1000";
    autoStart = false;
  };
  systemd.services."podman-photoprism-mariadb" = {
    serviceConfig = {
      Restart = "always";
    };
  };
  virtualisation.oci-containers.containers."sabnzbd" = {
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
      "/mnt/media:/storage:rw"
    ];
    labels = {
      "traefik.enable" = "true";
      "traefik.http.routers.sabnzbd.middlewares" = "chain-authelia@file";
      "traefik.http.routers.sabnzbd.rule" = "Host(`hey.hello.us`) && PathPrefix(`/sabnzbd`)";
      "traefik.http.routers.sabnzbd.tls.certresolver" = "htpc";
    };
    extraOptions = [
      "--network=default"
      "--network-alias=sabnzbd"
      "--log-driver=json-file"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
    autoStart = false;
  };
  systemd.services."podman-sabnzbd" = {
    serviceConfig = {
      Restart = "always";
      RuntimeMaxSec = 10;
    };
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
    extraOptions = [
      "--network=default"
      "--network-alias=traefik"
      "--log-driver=json-file"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
    autoStart = false;
  };
  systemd.services."podman-traefik" = {
    serviceConfig = {
      Restart = "always";
    };
  };
  virtualisation.oci-containers.containers."transmission" = {
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
      "/mnt/media:/storage:rw"
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
    extraOptions = [
      "--network=default"
      "--network-alias=transmission"
      "--dns=8.8.8.8"
      "--dns=8.8.4.4"
      "--privileged"
      "--cap-add=NET_ADMIN"
      "--device=/dev/net/tun:/dev/net/tun"
      "--log-driver=json-file"
      "--log-opt=compress=true"
      "--log-opt=max-file=3"
      "--log-opt=max-size=10m"
    ];
    autoStart = false;
  };
  systemd.services."podman-transmission" = {
    serviceConfig = {
      Restart = "on-failure";
    };
    startLimitBurst = 3;
  };

  # Networks
  systemd.services."create-podman-network-default" = {
    serviceConfig.Type = "oneshot";
    path = [ pkgs.podman ];
    script = ''
      podman network create default --opt isolate=true --ignore
    '';
    wantedBy = [
      "podman-jellyseerr.service"
      "podman-photoprism-mariadb.service"
      "podman-sabnzbd.service"
      "podman-traefik.service"
      "podman-transmission.service"
    ];
  };

  # Scripts
  up = writeShellScript "compose-up.sh" ''
    echo "TODO: Create resources."
  '';
  down = writeShellScript "compose-down.sh" ''
    echo "TODO: Remove resources."
  '';
}
