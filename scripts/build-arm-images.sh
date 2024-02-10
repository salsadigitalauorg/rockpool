#!/bin/bash

set -ex

ROCKPOOL_REPO=${ROCKPOOL_REPO:-https://github.com/salsadigitalauorg/rockpool}
ROCKPOOL_IMAGES_REPO=${ROCKPOOL_IMAGES_REPO:-ghcr.io/salsadigitalauorg/rockpool}
LAGOON_VERSION=${LAGOON_VERSION}

[[ "$(uname -s)" = "Darwin" ]] && sedbak=" .bak" || sedbak=""

function all () {
  k3s
  lagoon
  nfs_provisioner
}

function k3s () {
  pushd ..
  docker buildx bake k3s --progress=plain --push
  popd
}

function lagoon_download () {
  rm -rf lagoon
  if [ -z "${LAGOON_VERSION}" ]; then
    LAGOON_VERSION=$(curl -s https://api.github.com/repos/uselagoon/lagoon/releases/latest | jq -r '.tag_name')
  fi
  [ ! -d "lagoon" ] && \
    curl -Lo lagoon.tar.gz https://github.com/uselagoon/lagoon/archive/refs/tags/${LAGOON_VERSION}.tar.gz && \
    tar -xzf lagoon.tar.gz && mv lagoon-* lagoon && rm lagoon.tar.gz
  pushd lagoon
}

# Build lagoon images.
function lagoon_ssh () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_download
  fi
  dockerfile=services/ssh/Dockerfile
  sed -i${sedbak} 's/x86_64-linux-gnu/aarch64-linux-gnu/1' ${dockerfile}
  sed -i${sedbak} 's/\&\& \.\/configure \\/\&\& \.\/configure --build=aarch64-unknown-linux-gnu \\/1' ${dockerfile}
  sed -i${sedbak} 's/\/tini -o \/sbin\/tini /\/tini-arm64 -o \/sbin\/tini /1' ${dockerfile}
  docker buildx build --platform linux/arm64 --file ${dockerfile} \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/ssh:${LAGOON_VERSION} \
    --push .
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon () {
  lagoon_download
  lagoon_ssh true
  popd
}

# Build nfs-provisioner image.
function nfs_provisioner () {
  pushd ..
  docker buildx bake nfs-provisioner --progress=plain --push
  popd
}

target=${1:-all}

mkdir -p build
cd build

$target
