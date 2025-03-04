{ pkgs, lib, config, ... }:

{
  # Runtime
  virtualisation.podman = {
    enable = true;
    autoPrune.enable = true;
    dockerCompat = true;
  };

  # Enable container name DNS for all Podman networks.
  networking.firewall.interfaces = let
    matchAll = if !config.networking.nftables.enable then "podman+" else "podman*";
  in {
    "${matchAll}".allowedUDPPorts = [ 53 ];
  };

  virtualisation.oci-containers.backend = "podman";

  # Containers
  virtualisation.oci-containers.containers."test-auto-start" = {
    image = "nginx:latest";
    labels = {
      "compose2nix.settings.autoStart" = "true";
    };
    log-driver = "journald";
    extraOptions = [
      "--network-alias=auto-start"
      "--network=test_default"
    ];
  };
  systemd.services."podman-test-auto-start" = {
    serviceConfig = {
      Restart = lib.mkOverride 90 "always";
    };
    after = [
      "podman-network-test_default.service"
    ];
    requires = [
      "podman-network-test_default.service"
    ];
    partOf = [
      "podman-compose-test-root.target"
    ];
    wantedBy = [
      "podman-compose-test-root.target"
    ];
  };
  virtualisation.oci-containers.containers."test-default-no-auto-start" = {
    image = "nginx:latest";
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--network-alias=default-no-auto-start"
      "--network=test_default"
    ];
  };
  systemd.services."podman-test-default-no-auto-start" = {
    serviceConfig = {
      Restart = lib.mkOverride 90 "always";
    };
    after = [
      "podman-network-test_default.service"
    ];
    requires = [
      "podman-network-test_default.service"
    ];
  };
  virtualisation.oci-containers.containers."test-no-auto-start" = {
    image = "nginx:latest";
    labels = {
      "compose2nix.settings.autoStart" = "false";
    };
    log-driver = "journald";
    autoStart = false;
    extraOptions = [
      "--network-alias=no-auto-start"
      "--network=test_default"
    ];
  };
  systemd.services."podman-test-no-auto-start" = {
    serviceConfig = {
      Restart = lib.mkOverride 90 "always";
    };
    after = [
      "podman-network-test_default.service"
    ];
    requires = [
      "podman-network-test_default.service"
    ];
  };

  # Networks
  systemd.services."podman-network-test_default" = {
    path = [ pkgs.podman ];
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStop = "podman network rm -f test_default";
    };
    script = ''
      podman network inspect test_default || podman network create test_default
    '';
    partOf = [ "podman-compose-test-root.target" ];
    wantedBy = [ "podman-compose-test-root.target" ];
  };

  # Root service
  # When started, this will automatically create all resources and start
  # the containers. When stopped, this will teardown all resources.
  systemd.targets."podman-compose-test-root" = {
    unitConfig = {
      Description = "Root target generated by compose2nix.";
    };
  };
}
