# rockpool
[![Release](https://img.shields.io/github/v/release/yusufhm/rockpool)](https://github.com/yusufhm/rockpool/releases/latest)

rockpool is a CLI tool aiming to set up a local [Lagoon](https://github.com/uselagoon/lagoon) instance as painlessly as possible.

## Requirements

The following tools are needed for rockpool to work:
- [Docker](https://docs.docker.com/get-docker/)
- [k3d](https://github.com/k3d-io/k3d/#get)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [helm](https://helm.sh/docs/intro/install/)
- [lagoon](https://github.com/uselagoon/lagoon-cli#install)

## Install

```sh
# Mac Intel
curl -L -o rockpool https://github.com/yusufhm/rockpool/releases/latest/download/rockpool-Darwin-x86_64
# Mac M1
curl -L -o rockpool https://github.com/yusufhm/rockpool/releases/latest/download/rockpool-Darwin-aarch64
# Linux
curl -L -o rockpool https://github.com/yusufhm/rockpool/releases/latest/download/rockpool-$(uname -s)-$(uname -m)

chmod +x rockpool
mv rockpool /usr/local/bin/rockpool
```

## Usage

```sh
rockpool up
```

### What it does

* Creates two clusters:
  * controller - contains most of the components of the platform:
    * mailhog
    * ingress-nginx
    * cert-manager - for self-signed certificates, especially for harbor
    * Gitea - instead of Gitlab, since the latter consumes too much resources, and it was conflicting with one of the Lagoon Core components
    * Harbor
    * Lagoon core
  * target-1 - simply has Lagoon Remote installed
* Sets up some development defaults:
  * Configures keycloak to use mailhog, as per the [docs](https://docs.lagoon.sh/installing-lagoon/lagoon-core/#configure-keycloak)
  * Adds dns records to the target clusters' coredns config so it can access required services from the controller cluster
  * Registers the target clusters as remotes via the Lagoon API
  * Creates a self-signed certificate for Harbor on the controller and installs it on the target clusters


### Further usage

A number of flags can be used when creating the pool, as can be seen in the help:
```
$ rockpool up -h
up is for creating or starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool up controller target-1'

Usage:
  rockpool up [cluster-name...] [flags]

Flags:
  -h, --help                            help for up
  -l, --lagoon-base-url string          The base Lagoon url of the cluster;
                                        all Lagoon services will be created as subdomains of this url, e.g,
                                        ui.lagoon.rockpool.k3d.local, harbor.lagoon.rockpool.k3d.local
                                         (default "lagoon.rockpool.k3d.local")
      --rendered-template-path string   The directory where rendered template files are placed
                                         (default "/var/folders/lq/gql2jg193ndbp64b2z32qp8r0000gn/T/rockpool/rendered")
      --upgrade-components strings      A list of components to upgrade, e.g, ingress-nginx,harbor
  -u, --url string                      The base url of rockpool; ancillary services will be created
                                        as subdomains of this url, e.g, gitlab.rockpool.k3d.local
                                         (default "rockpool.k3d.local")

Global Flags:
  -n, --cluster-name string   The name of the cluster (default "rockpool")
  -t, --targets int           Number of targets (lagoon remotes) to create (default 1)
```

There are also other commands for controlling the platform:
```
$ rockpool
Usage:
  rockpool [command] [flags]
  rockpool [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  down        Stop the clusters and delete them
  help        Help about any command
  restart     Restart the clusters
  start       Start the clusters
  status      View the status of the clusters
  stop        Stop the clusters
  up          Create and/or start the clusters

Flags:
  -n, --cluster-name string   The name of the cluster (default "rockpool")
  -h, --help                  help for rockpool
  -t, --targets int           Number of targets (lagoon remotes) to create (default 1)

Use "rockpool [command] --help" for more information about a command.
```
