#!/bin/bash

set -ex

ROCKPOOL_REPO=${ROCKPOOL_REPO:-https://github.com/salsadigitalauorg/rockpool}
ROCKPOOL_IMAGES_REPO=${ROCKPOOL_IMAGES_REPO:-ghcr.io/salsadigitalauorg/rockpool}
KEYCLOAK_VERSION=${KEYCLOAK_VERSION:-16.1.1}
LAGOON_VERSION=${LAGOON_VERSION:-v2.12.0}

[[ "$(uname -s)" = "Darwin" ]] && sedbak=" .bak" || sedbak=""

function all () {
  k3s
  keycloak
  lagoon
  nfs_provisioner
}

function k3s () {
  pushd ..
  docker buildx bake k3s --progress=plain --push
  popd
}

# Build keycloak image.
function keycloak () {
  [ ! -d "keycloak-containers" ] && git clone https://github.com/keycloak/keycloak-containers.git keycloak-containers
  pushd keycloak-containers
  git checkout -- .
  git clean -fd .
  git checkout ${KEYCLOAK_VERSION}
  pushd server
  docker buildx build --platform linux/arm64 \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/keycloak:${KEYCLOAK_VERSION} \
    --push .
  popd
  popd
}

function lagoon_clone () {
  [ ! -d "lagoon" ] && git clone https://github.com/uselagoon/lagoon.git
  pushd lagoon
  git checkout -- .
  git clean -fd .
  git checkout ${LAGOON_VERSION}
}

# Build lagoon images.
function lagoon_keycloak () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/keycloak
  sed -i${sedbak} 's/FROM jboss\/keycloak/FROM ghcr\.io\/salsadigitalauorg\/rockpool\/keycloak/1' Dockerfile
  sed -i${sedbak} 's/${TINI_VERSION}\/tini/${TINI_VERSION}\/tini\-arm64/1' Dockerfile
  sed -i${sedbak} 's/\/var\/cache\/yum/\/var\/cache\/yum \&\& ln -s \/usr\/bin\/python2 \/usr\/bin\/python/1' Dockerfile
  docker buildx build --platform linux/arm64 \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/keycloak:${LAGOON_VERSION} \
    --push .
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon_broker_single () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/broker-single
  docker buildx build --platform linux/arm64 \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/broker-single:latest \
    --push .
  popd
}

function lagoon_broker () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/broker
  docker buildx build --platform linux/arm64 \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --build-arg IMAGE_REPO=${ROCKPOOL_IMAGES_REPO}/lagoon \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/broker:${LAGOON_VERSION} \
    --push .
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon () {
  lagoon_clone
  lagoon_keycloak true
  lagoon_broker_single true
  lagoon_broker true
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
