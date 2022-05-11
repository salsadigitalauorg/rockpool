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
curl -L -o rockpool https://github.com/yusufhm/rockpool/releases/latest/download/rockpool-$(uname -s)-$(uname -m)

chmod +x rockpool
mv rockpool /usr/local/bin/rockpool
```

## Usage

### Set up the platform

```sh
rockpool up
```
If there are failures, the same command can be run over and over again until it all succeeds. Failures could currently occur for a number of reasons, such as

- Waiting for ingress to be created
- Waiting for a certificate to be created
- Pods not yet ready

### Create a Lagoon project
**NOTE** on using Lagoon CLI:
> Currently the Lagoon CLI is built with `CGO_ENABLED=0`, which means that DNS lookups do not use the MacOs `/etc/resolver/*` files - see [here](https://github.com/golang/go/issues/12524#issuecomment-1006174901) - which means that `lagoon` commands interacting with the local instance will fail with an error similar to the following:
>
> ```Error: Post "http://api.lagoon.rockpool.k3d.local/graphql": dial tcp: lookup api.lagoon.rockpool.k3d.local on x.x.x.x:53: no such host```
>
> To get around that, the current option is to build the Lagoon CLI locally on MacOs with `CGO_ENABLED=1`:
> ```
> git clone git@github.com:uselagoon/lagoon-cli.git ~/go/src/github.com/uselagoon/lagoon-cli
> cd ~/go/src/github.com/uselagoon/lagoon-cli
> GO111MODULE=on CGO_ENABLED=1 go build -ldflags="-s -w" -o ~/go/bin/lagoon -v
> ```
> The built binary can now be used with rockpool:
> ```
> ~/go/bin/lagoon --lagoon rockpool list projects
> ```

The `rockpool up` command creates a test repository in gitea at `http://gitea.rockpool.k3d.local/rockpool/test.git` and a config for the Lagoon CLI. The test project can therefore be added to Lagoon using the following:

```sh
lagoon --lagoon rockpool add project \
  --gitUrl http://gitea.rockpool.k3d.local/rockpool/test.git \
  --openshift 1 \
  --productionEnvironment main \
  --branches "^(main|develop)$" \
  --project rockpool-test
```

Push some code to the test repository:
```sh
git clone https://github.com/lagoon-examples/drupal9-base.git rockpool-test && cd $_
git remote remove origin
git remote add origin http://gitea.rockpool.k3d.local/rockpool/test.git
git push -u origin main
```

Deploy the `main` environment:

```sh
lagoon --lagoon rockpool deploy branch \
  --project rockpool-test \
  --branch main
```

You can follow the progress of the deployment using the following:
```sh
kubectl --kubeconfig ~/.k3d/kubeconfig-rockpool-target-1.yaml \
  -n rockpool-test-main \
  logs -f -l lagoon.sh/jobType=build
```

## How it works

The `rockpool up` command:
* Creates two clusters:
  * controller - contains most of the components of the platform:
    * mailhog
    * ingress-nginx
    * cert-manager - for self-signed certificates, especially for harbor
    * Gitea - instead of Gitlab, since the latter consumes too much resources, and it was conflicting with one of the Lagoon Core components
    * Harbor
    * Lagoon core
  * target-1 - contains Lagoon Remote & [nfs provisioner](https://github.com/kubernetes-sigs/nfs-ganesha-server-and-external-provisioner)
* Sets up some development defaults:
  * Configures keycloak to use mailhog, as per the [docs](https://docs.lagoon.sh/installing-lagoon/lagoon-core/#configure-keycloak)
  * Adds dns records to the target clusters' coredns config so it can access required services from the controller cluster
  * Registers the target clusters as remotes via the Lagoon API
  * Creates a self-signed certificate for Harbor on the controller and installs it on the target clusters


## Further usage

A number of flags can be used when creating the pool, as can be seen in the help:
```
$ rockpool up -h
up is for creating or starting all the clusters, or the ones
specified in the arguments, e.g, 'rockpool up controller target-1'

Usage:
  rockpool up [name...] [flags]

Flags:
  -h, --help                         help for up
  -k, --ssh-key string               The ssh key to add to the lagoonadmin user. If empty, rockpool tries
                                     to use ~/.ssh/id_ed25519.pub first, then ~/.ssh/id_rsa.pub.

  -t, --targets int                  Number of targets (lagoon remotes) to create (default 1)
      --upgrade-components strings   A list of components to upgrade, e.g, ingress-nginx,harbor
  -u, --url string                   The base url of rockpool; ancillary services will be created
                                     as subdomains of this url, e.g, gitlab.rockpool.k3d.local
                                      (default "rockpool.k3d.local")

Global Flags:
  -n, --name string   The name of the platform (default "rockpool")
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
  -h, --help          help for rockpool
  -n, --name string   The name of the platform (default "rockpool")

Use "rockpool [command] --help" for more information about a command.
```
