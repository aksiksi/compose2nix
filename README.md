# compose2nix

[![Test](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml)
[![NixOS](https://github.com/aksiksi/compose2nix/actions/workflows/nixos.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/nixos.yml)
[![codecov](https://codecov.io/gh/aksiksi/compose2nix/graph/badge.svg)](https://codecov.io/gh/aksiksi/compose2nix)
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

1. Use in a Nix shell:
    ```bash
    nix shell github:aksiksi/compose2nix
    compose2nix -h
    ```
2. Run using `nix run`:
    ```
    nix run github:aksiksi/compose2nix -- -h
    ```
3. Install the Flake and add the following to your NixOS config:
    ```nix
    compose2nix.url = "github:aksiksi/compose2nix";
    compose2nix.inputs.nixpkgs.follows = "nixpkgs";

    environment.systemPackages = [
      compose2nix.packages.x86_64-linux.default
    ];
    ```
<!-- LINT.OnChange(version) -->
4. Install the command using `go`:
    ```
    go install github.com/aksiksi/compose2nix@v0.1.9
    ```
<!-- LINT.ThenChange(flake.nix:version, main.go:version) -->
5. Clone this repo and run `make build`.

Run `compose2nix`. Note that project must either be passed in **or** set in the Compose file's top-level "name".

```bash
compose2nix -project=myproject
```

By default, the tool looks for `docker-compose.yml` in the **current directory** and outputs the NixOS config to `docker-compose.nix`.

## Roadmap

- [x] Basic implementation
- [x] Support for most common Docker Compose features
- [x] Support for using secret environment files

## Docs

### Sample

* Input: https://github.com/aksiksi/compose2nix/blob/main/testdata/compose.yml
* Output (Docker): https://github.com/aksiksi/compose2nix/blob/main/testdata/TestBasic.docker.nix
* Output (Podman): https://github.com/aksiksi/compose2nix/blob/main/testdata/TestBasic.podman.nix

### Working with Secrets

#### [agenix](https://github.com/ryantm/agenix)

`agenix` works by decrypting secrets and placing them in `/run/agenix/`. To feed this into your Nix config:

1. Place all secret env variables in the encrypted env file (e.g., `my-env-file.env`).
2. Mark the decrypted env file as readable by the user running `compose2nix`.
3. Run `compose2nix` with the env file path(s) and set `-include_env_files=true`:

   ```
   compose2nix --env_files=/run/agenix/my-env-file.env --include_env_files=true
   ```

> [!NOTE]
> If you also want to ensure that you only include env files in the output Nix config, set `-env_files_only=true`.

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

#### Update a Container

1. Pull the latest image for the container (requires [`jq`](https://jqlang.github.io/jq/)):

```
sudo podman pull $(sudo podman inspect myproject-myservice | jq -r .[0].ImageName)
```

2. Restart the service:

```
sudo systemctl restart podman-myproject-myservice.service
```

#### **Podman**: Auto-update containers

1. Add a `io.containers.autoupdate=registry` label to each Compose service you want to have auto-updated.
    * Make sure to use a **fully-qualified** image path (e.g., `docker.io/repo/image`). Otherwise, Podman will fail to start the container.
2. Run `sudo podman auto-update --dry-run` to see which containers would get updated. Omit `--dry-run` to update & restart services.

You can optionally enable a Podman-provided timer that runs the command above once per day at midnight (by default):

```nix
# Enable the existing timer unit.
systemd.timers."podman-auto-update".wantedBy = [ "timers.target" ];
```

See this page for details: https://docs.podman.io/en/latest/markdown/podman-auto-update.1.html

#### `docker compose down`

By default, this will only remove networks.

```
sudo systemctl stop podman-compose-myservice-root.target
```

##### Remove volumes

You can do one of the following:

1. Re-generate your NixOS config with: `-remove_volumes=true`
2. Run `sudo podman volume prune` to manually cleanup unreferenced volumes

#### `docker compose up`

```
sudo systemctl start podman-compose-myservice-root.target
```

### NixOS Version Support Policy

I always aim to support the **latest** stable version of NixOS (24.05 at the
time of writing). As a result, some NixOS unstable options are not used.

If the option has a strong usecase, I am open to adding a CLI flag that can be
deprecated once the option is stable.

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
  -default_stop_timeout duration
        default stop timeout for generated container services. (default 1m30s)
  -env_files string
        one or more comma-separated paths to .env file(s).
  -env_files_only
        only use env file(s) in the NixOS container definitions.
  -generate_unused_resources
        if set, unused resources (e.g., networks) will be generated even if no containers use them.
  -ignore_missing_env_files
        if set, missing env files will be ignored.
  -include_env_files
        include env files in the NixOS container definition.
  -inputs string
        one or more comma-separated path(s) to Compose file(s). (default "docker-compose.yml")
  -output string
        path to output Nix file. (default "docker-compose.nix")
  -project string
        project name used as a prefix for generated resources. this overrides any top-level "name" set in the Compose file(s).
  -remove_volumes
        if set, volumes will be removed on systemd service stop.
  -root_path string
        root path to use for any relative paths in the Compose file (e.g., volumes). if unset, the current working directory will be used.
  -runtime string
        one of: ["podman", "docker"]. (default "podman")
  -service_include string
        regex pattern for services to include.
  -use_compose_log_driver
        if set, always use the Docker Compose log driver.
  -use_upheld_by
        if set, upheldBy will be used for service dependencies (NixOS 24.05+).
  -version
        display version and exit
  -write_nix_setup
        if true, Nix setup code is written to output (runtime, DNS, autoprune, etc.) (default true)
```

### Supported Docker Compose Features

If a feature is missing, please feel free to [create an issue](https://github.com/aksiksi/compose2nix/issues/new). In theory, any Compose feature can be supported because `compose2nix` uses the same library as the Docker CLI under the hood.

#### [`services`](https://docs.docker.com/compose/compose-file/05-services/)

|   |     | Notes |
|---|:---:|-------|
| [`image`](https://docs.docker.com/compose/compose-file/05-services/#image) | ✅ | |
| [`container_name`](https://docs.docker.com/compose/compose-file/05-services/#container_name) | ✅ | |
| [`environment`](https://docs.docker.com/compose/compose-file/05-services/#environment) | ✅ | |
| [`volumes`](https://docs.docker.com/compose/compose-file/05-services/#volumes) | ✅ | |
| [`labels`](https://docs.docker.com/compose/compose-file/05-services/#labels) | ✅ | |
| [`ports`](https://docs.docker.com/compose/compose-file/05-services/#ports) | ✅ | |
| [`dns`](https://docs.docker.com/compose/compose-file/05-services/#dns) | ✅ | |
| [`cap_add/cap_drop`](https://docs.docker.com/compose/compose-file/05-services/#cap_add) | ✅ | |
| [`logging`](https://docs.docker.com/compose/compose-file/05-services/#logging) | ✅ | |
| [`depends_on`](https://docs.docker.com/compose/compose-file/05-services/#depends_on) | ⚠️ | Only short syntax is supported. |
| [`restart`](https://docs.docker.com/compose/compose-file/05-services/#restart) | ✅ | |
| [`deploy.restart_policy`](https://docs.docker.com/compose/compose-file/deploy/#restart_policy) | ✅ | |
| [`deploy.resources`](https://docs.docker.com/compose/compose-file/deploy/#resources) | ✅ | |
| [`devices`](https://docs.docker.com/compose/compose-file/05-services/#devices) | ✅ | |
| [`networks.aliases`](https://docs.docker.com/compose/compose-file/05-services/#aliases) | ✅ | |
| [`networks.ipv*_address`](https://docs.docker.com/compose/compose-file/05-services/#ipv4_address-ipv6_address) | ✅ | |
| [`network_mode`](https://docs.docker.com/compose/compose-file/05-services/#network_mode) | ✅ | |
| [`privileged`](https://docs.docker.com/compose/compose-file/05-services/#privileged) | ✅ | |
| [`extra_hosts`](https://docs.docker.com/compose/compose-file/05-services/#extra_hosts) | ✅ | |
| [`sysctls`](https://docs.docker.com/compose/compose-file/05-services/#sysctls) | ✅ | |
| [`shm_size`](https://docs.docker.com/compose/compose-file/05-services/#shm_size) | ✅ | |
| [`runtime`](https://docs.docker.com/compose/compose-file/05-services/#runtime) | ✅ | |
| [`security_opt`](https://docs.docker.com/compose/compose-file/05-services/#security_opt) | ✅ | |
| [`command`](https://docs.docker.com/compose/compose-file/05-services/#command) | ✅ | |
| [`healthcheck`](https://docs.docker.com/compose/compose-file/05-services/#healthcheck) | ✅ | |
| [`hostname`](https://docs.docker.com/compose/compose-file/05-services/#hostname) | ✅ | |
| [`mac_address`](https://docs.docker.com/compose/compose-file/05-services/#mac_address) | ✅ | |

#### [`networks`](https://docs.docker.com/compose/compose-file/06-networks/)

|   |     |
|---|:---:|
| [`labels`](https://docs.docker.com/compose/compose-file/06-networks/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/06-networks/#name) | ✅ |
| [`driver`](https://docs.docker.com/compose/compose-file/06-networks/#driver) | ✅ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/06-networks/#driver_opts) | ✅ |
| [`ipam`](https://docs.docker.com/compose/compose-file/06-networks/#ipam) | ✅ |
| [`external`](https://docs.docker.com/compose/compose-file/06-networks/#external) | ✅ |
| [`internal`](https://docs.docker.com/compose/compose-file/06-networks/#internal) | ✅ |

#### [`volumes`](https://docs.docker.com/compose/compose-file/07-volumes/)

|   |     |
|---|:---:|
| [`driver`](https://docs.docker.com/compose/compose-file/07-volumes/#driver) | ✅ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/07-volumes/#driver_opts) | ✅ |
| [`labels`](https://docs.docker.com/compose/compose-file/07-volumes/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/07-volumes/#name) | ✅ |
| [`external`](https://docs.docker.com/compose/compose-file/07-volumes/#external) | ✅ |

#### Misc

* [`name`](https://docs.docker.com/compose/compose-file/04-version-and-name/#name-top-level-element) - ✅
