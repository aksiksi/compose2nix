# compose2nix

[![codecov](https://codecov.io/gh/aksiksi/compose2nix/graph/badge.svg)](https://codecov.io/gh/aksiksi/compose2nix)
[![test](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/aksiksi/compose2nix.svg)](https://pkg.go.dev/github.com/aksiksi/compose2nix)

A tool to automatically generate a NixOS config from a Docker Compose project.

## Overview

### Why?

Running a Docker Compose stack/project on NixOS is not well supported. One approach is to define a systemd service that runs `docker-compose up` on start and `docker compose down` on stop.

But with this approach, changes to individual services are not visible to NixOS, which means that NixOS will need to restart the systemd service on **any** change to the Compose file. This can be mitigated by defining a systemd reload handler, but it still is finicky to work with and will always remain opaque to NixOS.

To top it all off, using Docker Compose on NixOS is fairly redundant as the features you get with Compose are available natively on NixOS.

### How?

`compose2nix` takes your existing Docker Compose file(s) and converts each YAML service definition into a [`oci-container`](https://search.nixos.org/options?query=virtualisation.oci-containers) config. The tool also sets up systemd services to create all networks and volumes that are part of the Compose project. Since `compose2nix` uses the same library that the Docker CLI relies on under the hood, you also get Compose file validation and syntax checking "for free".

### Benefits

1. Supports both Docker and Podman out of the box.
2. Each Compose service maps into a systemd service that is natively managed by NixOS.
3. A change to one container service only impacts that container and any of its dependents.
4. Generated systemd services can be extended from your NixOS config.
5. `compose2nix` supports setting additional systemd service and unit options through Docker Compose labels (search for the `compose2nix.systemd.` label in the samples).

## Quickstart

Install the `compose2nix` CLI via one of the following methods:

1. Install the command using `go`:
    ```
    go install github.com/aksiksi/compose2nix@v0.1.4
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

## Roadmap

- [x] Basic implementation
- [x] Support for most common Docker Compose features
- [ ] Support for using secret environment files
- [ ] ???

## Docs

### Patterns

In this case, the project is called `myproject` and the service name is `myservice`. Replace `podman` with `docker` if using the Docker runtime.

#### List all services

```
sudo systemctl list-units podman-*
```

#### List all services in a project

```
sudo systemctl list-units *myservice*
```

#### Restart a service

Note: if the Compose service has a `container_name` set, then the systemd service will not include the project name.

```
sudo systemctl restart podman-myproject-myservice.service
```

#### `docker compose down`

By default, this will only remove networks.

```
sudo systemctl stop podman-compose-myservice-root.service
```

##### Remove volumes

Either:

1. Re-generate your NixOS config with `-remove_volumes=true`.
2. Run `sudo podman volume prune`.

#### `docker compose up`

```
sudo systemctl start podman-compose-myservice-root.service
```

### Usage

```
$ compose2nix -h
Usage of compose2nix:
  -auto_start
        auto-start setting for generated service(s). this applies to all services, not just containers. (default true)
  -check_systemd_mounts
        if set, volume paths will be checked against systemd mount paths on the current machine and marked as container dependencies.
  -create_root_target
        if set, a root systemd target will be created, which when stopped tears down all resources. (default true)
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
        project name used as a prefix for generated resources. this overrides any top-level "name" set in the Compose file(s).
  -remove_volumes
        if set, volumes will be removed on systemd service stop.
  -runtime string
        one of: ["podman", "docker"]. (default "podman")
  -service_include string
        regex pattern for services to include.
  -use_compose_log_driver
        if set, always use the Docker Compose log driver.
  -version
        display version and exit
```

### Sample

* Input: https://github.com/aksiksi/compose2nix/blob/main/testdata/docker-compose.yml
* Output (Docker): https://github.com/aksiksi/compose2nix/blob/main/testdata/TestDocker_out.nix
* Output (Podman): https://github.com/aksiksi/compose2nix/blob/main/testdata/TestPodman_out.nix

### Supported Docker Compose Features

If a feature is missing, please feel free to [create an issue](https://github.com/aksiksi/compose2nix/issues/new). In theory, any Compose feature can be supported because `compose2nix` uses the same library as the Docker CLI under the hood.

#### [`services`](https://docs.docker.com/compose/compose-file/05-services/)

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

#### [`networks`](https://docs.docker.com/compose/compose-file/06-networks/)

|   |     |
|---|:---:|
| [`labels`](https://docs.docker.com/compose/compose-file/06-networks/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/06-networks/#name) | ❌ |
| [`driver`](https://docs.docker.com/compose/compose-file/06-networks/#driver) | ❌ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/06-networks/#driver_opts) | ❌ |
| [`external`](https://docs.docker.com/compose/compose-file/06-networks/#external) | ❌ |
| [`internal`](https://docs.docker.com/compose/compose-file/06-networks/#internal) | ❌ |

#### [`volumes`](https://docs.docker.com/compose/compose-file/07-volumes/)

|   |     |
|---|:---:|
| [`driver`](https://docs.docker.com/compose/compose-file/07-volumes/#driver) | ✅ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/07-volumes/#driver_opts) | ✅ |
| [`labels`](https://docs.docker.com/compose/compose-file/07-volumes/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/07-volumes/#name) | ❌ |
| [`external`](https://docs.docker.com/compose/compose-file/07-volumes/#external) | ❌ |

#### Misc

* [`name`](https://docs.docker.com/compose/compose-file/04-version-and-name/#name-top-level-element) - ✅
