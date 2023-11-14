# compose2nix

[![codecov](https://codecov.io/gh/aksiksi/compose2nix/graph/badge.svg)](https://codecov.io/gh/aksiksi/compose2nix)
[![test](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/aksiksi/compose2nix.svg)](https://pkg.go.dev/github.com/aksiksi/compose2nix)

A tool to automatically generate a NixOS config from a Docker Compose project.

## Quickstart

Install the `compose2nix` CLI via one of the following methods:

1. Install the command using `go`:
    ```
    go install github.com/aksiksi/compose2nix@v0.1.2
    ```
2. Clone this repo and run `make build`.
3. Install the Flake and add the following to your NixOS config:
    ```nix
    environment.systemPackages = with pkgs; [
      compose2nix.packages.x86_64-linux.default
    ];
    ```

Run `compose2nix`. Note that project must either be passed in **or** set in the Compose file's top-level "name".

```bash
compose2nix -project=myproject
```

By default, the tool looks for `docker-compose.yml` in the **current directory** and outputs the NixOS config to `docker-compose.nix`.

## Supported Features

### [`services`](https://docs.docker.com/compose/compose-file/05-services/)

|   |     |
|---|:---:|
| [`image`](https://docs.docker.com/compose/compose-file/05-services/#image) | ✅ |
| [`container_name`](https://docs.docker.com/compose/compose-file/05-services/#container_name) | ✅ |
| [`environment`](https://docs.docker.com/compose/compose-file/05-services/#environment) | ✅ |
| [`volumes`](https://docs.docker.com/compose/compose-file/05-services/#volumes) | ✅ |
| [`labels`](https://docs.docker.com/compose/compose-file/05-services/#labels) | ✅ |
| [`ports`](https://docs.docker.com/compose/compose-file/05-services/#ports) | ✅ |
| [`dns`](https://docs.docker.com/compose/compose-file/05-services/#dns) | ✅ |
| [`cap_add/cap_drop`](https://docs.docker.com/compose/compose-file/05-services/#cap_add) | ✅ |
| [`logging`](https://docs.docker.com/compose/compose-file/05-services/#logging) | ✅ |
| [`restart`](https://docs.docker.com/compose/compose-file/05-services/#restart) | ✅ |
| [`deploy.restart_policy`](https://docs.docker.com/compose/compose-file/deploy/#restart_policy) | ✅ |
| [`devices`](https://docs.docker.com/compose/compose-file/05-services/#devices) | ✅ |
| [`networks.aliases`](https://docs.docker.com/compose/compose-file/05-services/#aliases) | ✅ |
| [`network_mode`](https://docs.docker.com/compose/compose-file/05-services/#network_mode) | ✅ |
| [`privileged`](https://docs.docker.com/compose/compose-file/05-services/#privileged) | ✅ |
| [`extra_hosts`](https://docs.docker.com/compose/compose-file/05-services/#extra_hosts) | ✅ |
| [`sysctls`](https://docs.docker.com/compose/compose-file/05-services/#sysctls) | ✅ |

### [`networks`](https://docs.docker.com/compose/compose-file/06-networks/)

|   |     |
|---|:---:|
| [`labels`](https://docs.docker.com/compose/compose-file/06-networks/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/06-networks/#name) | ❌ |
| [`driver`](https://docs.docker.com/compose/compose-file/06-networks/#driver) | ❌ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/06-networks/#driver_opts) | ❌ |
| [`external`](https://docs.docker.com/compose/compose-file/06-networks/#external) | ❌ |
| [`internal`](https://docs.docker.com/compose/compose-file/06-networks/#internal) | ❌ |

### [`volumes`](https://docs.docker.com/compose/compose-file/07-volumes/)

|   |     |
|---|:---:|
| [`driver`](https://docs.docker.com/compose/compose-file/07-volumes/#driver) | ✅ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/07-volumes/#driver_opts) | ✅ |
| [`labels`](https://docs.docker.com/compose/compose-file/07-volumes/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/07-volumes/#name) | ❌ |
| [`external`](https://docs.docker.com/compose/compose-file/07-volumes/#external) | ❌ |

### Misc

* [`name`](https://docs.docker.com/compose/compose-file/04-version-and-name/#name-top-level-element) - ✅

## Options

```
$ compose2nix -h
Usage of compose2nix:
  -auto_start
        auto-start setting for generated container(s). (default true)
  -check_systemd_mounts
        if set, volume paths will be checked against systemd mount paths on the current machine and marked as container dependencies.
  -create_root_service
        if set, a root systemd service will be created, which when stopped tears down all resources. (default true)
  -env_files string
        one or more comma-separated paths to .env file(s).
  -env_files_only
        only use env file(s) in the NixOS container definitions.
  -generate_unused_resources
        if set, unused resources (e.g., networks) will be generated even if no containers use them.
  -inputs string
        one or more comma-separated path(s) to Compose file(s). (default "docker-compose.yml")
  -output string
        path to output Nix file. (default "docker-compose.nix")
  -project string
        project name used as a prefix for generated resources.
  -project_separator string
        seperator for project prefix. (default "_")
  -remove_volumes
        if set, volumes will be removed on systemd service stop.
  -runtime string
        one of: ["podman", "docker"]. (default "podman")
  -service_include string
        regex pattern for services to include.
  -use_compose_log_driver
        if set, always use the Docker Compose log driver.
```
