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
      "/mnt/media/Books:/books:rw"
      "/var/volumes/jellyseerr:/app/config:rw"
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
  systemd.services."podman-jellyseerr" = {
    serviceConfig = {
      Restart = "on-failure";
      RestartSec = "5s";
    };
    startLimitBurst = 3;
    startLimitIntervalSec = 120;
    partOf = [ "podman-compose-myproject-root.target" ];
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
      "/mnt/media:/storage:rw"
      "/var/volumes/sabnzbd:/config:rw"
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
  systemd.services."podman-myproject-sabnzbd" = {
    serviceConfig = {
      Restart = "always";
      RuntimeMaxSec = 10;
    };
    unitConfig = {
      Description = "This is the sabnzbd container!";
    };
    partOf = [ "podman-compose-myproject-root.target" ];
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
      "/mnt/photos:/photos:rw"
      "/var/volumes/photoprism-mariadb:/var/lib/mysql:rw"
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
  systemd.services."podman-photoprism-mariadb" = {
    serviceConfig = {
      Restart = "always";
      RestartSec = "3m0s";
    };
    startLimitBurst = 10;
    startLimitIntervalSec = 86400;
    partOf = [ "podman-compose-myproject-root.target" ];
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
      "/mnt/media:/storage:rw"
      "/var/volumes/transmission/config:/config:rw"
      "/var/volumes/transmission/scripts:/scripts:rw"
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
      "--network-alias=transmission"
      "--network=myproject-something:alias=my-torrent-client"
      "--privileged"
    ];
  };
  systemd.services."podman-torrent-client" = {
    serviceConfig = {
      Restart = "on-failure";
    };
    startLimitBurst = 3;
    startLimitIntervalSec = 86400;
    partOf = [ "podman-compose-myproject-root.target" ];
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
  systemd.services."podman-traefik" = {
    serviceConfig = {
      Restart = "none";
    };
    partOf = [ "podman-compose-myproject-root.target" ];
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
    before = [
      "podman-myproject-sabnzbd.service"
    ];
    requiredBy = [
      "podman-myproject-sabnzbd.service"
    ];
    partOf = [ "podman-compose-myproject-root.target" ];
  };
  systemd.services."podman-network-myproject-something" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "${pkgs.podman}/bin/podman network rm -f myproject-something";
    };
    script = ''
      podman network inspect myproject-something || podman network create myproject-something --opt isolate=true --label=test-label=okay
    '';
    before = [
      "podman-torrent-client.service"
    ];
    requiredBy = [
      "podman-torrent-client.service"
    ];
    partOf = [ "podman-compose-myproject-root.target" ];
  };

  # Root service
  # When started, this will automatically create all resources and start
  # the containers. When stopped, this will teardown all resources.
  systemd.targets."podman-compose-myproject-root" = {
    unitConfig = {
      Description = "Root target generated by compose2nix.";
    };
    wants = [
      "podman-jellyseerr.service"
      "podman-myproject-sabnzbd.service"
      "podman-photoprism-mariadb.service"
      "podman-torrent-client.service"
      "podman-traefik.service"
      "podman-network-myproject-default.service"
      "podman-network-myproject-something.service"
    ];
  };
}
