# compose2nix

[![Test](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/test.yml)
[![NixOS](https://github.com/aksiksi/compose2nix/actions/workflows/nixos.yml/badge.svg)](https://github.com/aksiksi/compose2nix/actions/workflows/nixos.yml)
[![codecov](https://codecov.io/gh/aksiksi/compose2nix/graph/badge.svg)](https://codecov.io/gh/aksiksi/compose2nix)
[![Go Reference](https://pkg.go.dev/badge/github.com/aksiksi/compose2nix.svg)](https://pkg.go.dev/github.com/aksiksi/compose2nix)

A tool to automatically generate a NixOS config from a Docker Compose project.

## Overview

### Intro video

[![Rambly introduction video](https://img.youtube.com/vi/hCAFyzJ81Pg/hqdefault.jpg)](https://youtu.be/hCAFyzJ81Pg)

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

1. Run using `nix run` (**recommended**):
    ```
    # Latest
    nix run github:aksiksi/compose2nix -- -h

    # Specific version
    nix run github:aksiksi/compose2nix/v0.3.0 -- -h

    # Specific commit
    nix run github:aksiksi/compose2nix/0c38d282d6662fc902fca7ef5b33e889f9e3e59a -- -h
    ```
2. Install from `nixpkgs`:
    ```
    # NixOS config
    environment.systemPackages = [
      pkgs.compose2nix
    ];
    ```
3. Add the following to your `flake.nix`:
    ```nix
    compose2nix.url = "github:aksiksi/compose2nix";
    compose2nix.inputs.nixpkgs.follows = "nixpkgs";
    ```

    Optionally, you can pin to a specific version:
    ```nix
    compose2nix.url = "github:aksiksi/compose2nix/v0.3.0";
    ```

    You can then install the package by adding the following to your NixOS config:
    ```nix
    environment.systemPackages = [
      inputs.compose2nix.packages.x86_64-linux.default
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

#### [sops-nix](https://github.com/Mic92/sops-nix)

The `sops-nix` integration allows you to reference secrets that are already configured in your NixOS system.

> [!NOTE]
> This section assumes that:
>
> 1. `sops-nix` is already setup in your NixOS configuration
> 2. The secrets you want to use are already defined in your configuration

To use the `sops-nix` integration:

1. Add a `compose2nix.settings.sops.secrets` label with *comma-separated* secret names to your Compose services:

   ```yaml
   services:
     webapp:
       image: nginx:latest
       labels:
         - "compose2nix.settings.sops.secrets=example.env,some-folder/example-2.env"
   ```

2. Run `compose2nix` pointing to your *encrypted* secrets YAML file:

   ```bash
   compose2nix \
     --inputs docker-compose.yml \
     --sops_file ./secrets/secrets.yaml
   ```

This will then generate a NixOS configuration that references your existing sops secrets as environment files. Note that they'll be *appended* to env files passed during connfig generation.

```nix
virtualisation.oci-containers.containers."webapp" = {
  image = "nginx:latest";
  # ...
  environmentFiles = [
    "/etc/existing-file.env" # passed in via CLI

    # sops-nix secrets
    config.sops.secrets."example.env".path
    config.sops.secrets."folder/example-2.env".path
  ];
};
```

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

#### Auto-start services on boot

By default, all generated services will be started by systemd on boot.

You can override this behavior in two different ways:

1. **Disable auto-start for all services:** Re-generate your config with `-auto_start=false`.
2. **Disable or enable auto-start for a single service:** Add a Compose label to your service like this:

    ```yaml
    services:
      my-service:
        labels:
          # Enable
          - "compose2nix.settings.autoStart=true"
          # Disable
          - "compose2nix.settings.autoStart=false"
    ```

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

#### Compose Build spec

`compose2nix` has basic support for the Build spec. See [Supported Compose Features] below for details.

By default, a systemd service will be generated for _each_ container build. This is a
one-shot service that simply runs the build when started.

For example, if you have a service named `my-service` with a `build` set:

```
sudo systemctl start podman-build-my-service.service
```

Note that, until this is run, the container for `my-service` will **not be able to start** due to the missing image.

##### Auto-build

If you run the CLI with `-build=true`, the systemd service will be marked as a dependency for the service
container. This means that the build will be run before the container is started.

However, it is important to note that the build will be re-run on every restart of the root target or system.
This will result in the build image being updated (potentially).

### Nvidia GPU Support

1. Enable CDI support in your NixOS config:

```nix
{
  hardware.nvidia-container-toolkit.enable = true;
}
```

**Docker only:**

Make sure you are running Docker 25+:

```nix
{
  virtualisation.docker.package = pkgs.docker_25;
}
```

2. Pass in CDI devices via either `devices` **or** `deploy` (both map to the same thing under the hood):

```yaml
services:
  myservice:
    # ... other fields

    # Option 1
    devices:
      - nvidia.com/gpu=all
    # Option 2
    deploy:
      resources:
        reservations:
          devices:
            # Driver must be set to "cdi" - all others are ignored.
            - driver: cdi
              device_ids:
                - nvidia.com/gpu=all

    # Required for Podman.
    security_opt:
      - label=disable
```

### NixOS Version Support Policy

I always aim to support the **latest** stable version of NixOS (24.05 at the
time of writing). As a result, some NixOS unstable options are not used.

If the option has a strong usecase, I am open to adding a CLI flag that can be
deprecated once the option is stable.

### Known Issues

#### Manually stopping containers with UpheldBy

systemd does not differentiate between a manual unit stop and a unit stopped due
to a failure (i.e., in failed state). This means that if you stop a unit, it
will automatically be started by the service(s) it depends on.

Suppose you have the following Compose file:

```yaml
services:
  app:
    image: myname/app
    depends_on:
      - db
  db:
    image: postgres
```

If you *manually stop* the `app` systemd unit, the `db` unit will
**automatically* restart it due to the `UpheldBy` setting.

Discussion with the systemd team: https://github.com/systemd/systemd/issues/35636

#### Docker & multiple networks

If you are using the Docker runtime and a Compose service connects to multiple networks, you'll need to use v25+. Otherwise, the container service will fail to start.

You can pin the Docker version to v25 like so:

```nix
{
  virtualisation.docker.package = pkgs.docker_25;
}
```

Discussion: https://github.com/aksiksi/compose2nix/issues/24

#### Podman: Port forwarding in `internal` networks

For some reason, when you run a *rootful* Podman container in a network that is marked as `internal`, port forwarding to the host does not work. Podman seems to completely isolate the network from the external world - including the host network! Note that Docker claims to support this behavior out-of-the-box ([ref](https://docs.docker.com/reference/cli/docker/network/create/#internal)).

There is a workaround: remove the `internal` setting and set the network driver option `no_default_route=1` ([example](https://github.com/aksiksi/compose2nix/discussions/39#discussioncomment-10717015)).

```yaml
networks:
  my-network:
    driver: bridge
    driver_opts:
      no_default_route: 1 # <<< This is what prevents external network access.
    ipam:
      config:
        - subnet: 10.8.1.0/24
          gateway: 10.8.1.0
```

This will allow you to connect from the host, while also preventing internet access from within the container. 

This is where the check is done in Netavark: [link](https://github.com/containers/netavark/blob/cebebc70daec7010c4005798a7958b3b6be7151d/src/network/bridge.rs#L756)

### Supported Compose Features

If a feature is missing, please feel free to [create an issue](https://github.com/aksiksi/compose2nix/issues/new). In theory, any Compose feature can be supported because `compose2nix` uses the same library as the Docker CLI under the hood.

#### [`services`](https://docs.docker.com/compose/compose-file/05-services/)

|   |     | Notes |
|---|:---:|-------|
| [`image`](https://docs.docker.com/compose/compose-file/05-services/#image) | ✅ | |
| [`container_name`](https://docs.docker.com/compose/compose-file/05-services/#container_name) | ✅ | |
| [`environment`](https://docs.docker.com/compose/compose-file/05-services/#environment) | ✅ | |
| [`env_file`](https://docs.docker.com/compose/compose-file/05-services/#env_file) | ✅ | |
| [`volumes`](https://docs.docker.com/compose/compose-file/05-services/#volumes) | ✅ | Short and long syntax supported.|
| [`labels`](https://docs.docker.com/compose/compose-file/05-services/#labels) | ✅ | |
| [`ports`](https://docs.docker.com/compose/compose-file/05-services/#ports) | ✅ | |
| [`dns`](https://docs.docker.com/compose/compose-file/05-services/#dns) | ✅ | |
| [`cap_add/cap_drop`](https://docs.docker.com/compose/compose-file/05-services/#cap_add) | ✅ | |
| [`logging`](https://docs.docker.com/compose/compose-file/05-services/#logging) | ✅ | |
| [`depends_on`](https://docs.docker.com/compose/compose-file/05-services/#depends_on) | ⚠️  | Only short syntax is supported. |
| [`restart`](https://docs.docker.com/compose/compose-file/05-services/#restart) | ✅ | |
| [`deploy.restart_policy`](https://docs.docker.com/compose/compose-file/deploy/#restart_policy) | ✅ | |
| [`deploy.resources.limits`](https://docs.docker.com/compose/compose-file/deploy/#resources) | ✅ | |
| [`deploy.resources.reservations.cpus`](https://docs.docker.com/compose/compose-file/deploy/#cpus) | ✅ | |
| [`deploy.resources.reservations.memory`](https://docs.docker.com/compose/compose-file/deploy/#memory) | ✅ | |
| [`deploy.resources.reservations.devices`](https://docs.docker.com/compose/compose-file/deploy/#devices) | ⚠️  | Only CDI driver is supported. |
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
| [`entrypoint`](https://docs.docker.com/compose/compose-file/05-services/#entrypoint) | ✅ | |
| [`healthcheck`](https://docs.docker.com/compose/compose-file/05-services/#healthcheck) | ✅ | |
| [`hostname`](https://docs.docker.com/compose/compose-file/05-services/#hostname) | ✅ | |
| [`mac_address`](https://docs.docker.com/compose/compose-file/05-services/#mac_address) | ✅ | |
| [`user`](https://docs.docker.com/compose/compose-file/05-services/#user) | ✅ | |

#### [`networks`](https://docs.docker.com/compose/compose-file/06-networks/)

|   |     |
|---|:---:|
| [`labels`](https://docs.docker.com/compose/compose-file/06-networks/#labels) | ✅ |
| [`name`](https://docs.docker.com/compose/compose-file/06-networks/#name) | ✅ |
| [`driver`](https://docs.docker.com/compose/compose-file/06-networks/#driver) | ✅ |
| [`driver_opts`](https://docs.docker.com/compose/compose-file/06-networks/#driver_opts) | ✅ |
| [`enable_ipv6`](https://docs.docker.com/compose/compose-file/06-networks/#enable_ipv6) | ✅ |
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

#### [`build`](https://docs.docker.com/reference/compose-file/build/)

|   |     | Notes |
|---|:---:|-------|
| [`args`](https://docs.docker.com/reference/compose-file/build/#args) | ✅ | |
| [`tags`](https://docs.docker.com/reference/compose-file/build/#tags) | ✅ |
| [`context`](https://docs.docker.com/reference/compose-file/build/#context) | ⚠️  | Git repo is not supported |
| [`network`](https://docs.docker.com/reference/compose-file/build/#network) | ❌ |
| [`image`+`build`](https://docs.docker.com/reference/compose-file/build/#using-build-and-image) | ❌ |

#### Misc

* [`name`](https://docs.docker.com/compose/compose-file/04-version-and-name/#name-top-level-element) - ✅


### Usage

```
$ compose2nix -h
Usage of compose2nix:
  -auto_format
    	if true, Nix output will be formatted using "nixfmt" (must be present in $PATH).
  -auto_start
    	auto-start setting for generated service(s). this applies to all services, not just containers. (default true)
  -build
    	if set, generated container build systemd services will be enabled.
  -check_bind_mounts
    	if set, check that bind mount paths exist. this is useful if running the generated Nix code on the same machine.
  -check_systemd_mounts
    	if set, volume paths will be checked against systemd mount paths on the current machine and marked as container dependencies.
  -create_root_target
    	if set, a root systemd target will be created, which when stopped tears down all resources. (default true)
  -default_stop_timeout duration
    	default stop timeout for generated container services. (default 1m30s)
  -enable_option
    	generate a NixOS module option. this allows you to enable or disable the generated module from within your NixOS config. by default, the option will be named "options.[project_name]", but you can add a prefix using the "option_prefix" flag.
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
  -option_prefix string
    	Prefix for the option. If empty, the project name will be used as the option name. (e.g. custom.containers)
  -output string
    	path to output Nix file. (default "docker-compose.nix")
  -project string
    	project name used as a prefix for generated resources. this overrides any top-level "name" set in the Compose file(s).
  -remove_volumes
    	if set, volumes will be removed on systemd service stop.
  -root_path string
    	absolute path to use as the root for any relative paths in the Compose file (e.g., volumes, env files). defaults to the current working directory.
  -runtime string
    	one of: ["podman", "docker"]. (default "podman")
  -service_include string
    	regex pattern for services to include.
  -sops_file string
    	path to encrypted secrets YAML file (e.g., secrets.yaml). when set, secrets defined in compose services using "compose2nix.sops.secret=secret1,secret2" labels will be added as environmentFiles.
  -use_compose_log_driver
    	if set, always use the Docker Compose log driver.
  -use_upheld_by
    	if set, upheldBy will be used for service dependencies (NixOS 24.05+).
  -version
    	display version and exit
  -warnings_as_errors
    	if set, treat generator warnings as hard errors.
  -write_nix_setup
    	if true, Nix setup code is written to output (runtime, DNS, autoprune, etc.) (default true)
```

### Alternative Installation Methods

1. Use in a Nix shell:
    ```bash
    nix shell github:aksiksi/compose2nix
    ```
2. Install the command using `go`:
    ```
    go install github.com/aksiksi/compose2nix
    ```
3. Clone this repo and run `make build`.

