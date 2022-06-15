#!/bin/bash

set -ex

ROCKPOOL_REPO=${ROCKPOOL_REPO:-https://github.com/salsadigitalauorg/rockpool}
ROCKPOOL_IMAGES_REPO=${ROCKPOOL_IMAGES_REPO:-ghcr.io/salsadigitalauorg/rockpool}
KEYCLOAK_VERSION=${KEYCLOAK_VERSION:-7.0.1}
LAGOON_VERSION=${LAGOON_VERSION:-v2.5.0}

function all () {
  k3s
  keycloak
  lagoon
  nfs_provisioner
}

function k3s () {
  pushd ..
  docker buildx bake k3s --push
  popd
}

# Build keycloak image.
function keycloak () {
  [ ! -d "keycloak-containers" ] && git clone https://github.com/keycloak/keycloak-containers.git keycloak-containers
  pushd keycloak-containers && git checkout -- . && git clean -fd . && git checkout ${KEYCLOAK_VERSION} && pushd server
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/keycloak:${KEYCLOAK_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/keycloak:${KEYCLOAK_VERSION}
  popd +2
}

function lagoon_clone () {
  [ ! -d "lagoon" ] && git clone https://github.com/uselagoon/lagoon.git
  pushd lagoon
  git checkout -- . && git clean -fd . && git checkout ${LAGOON_VERSION}
}

# Build lagoon images.
function lagoon_keycloak () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/keycloak
  sed -i .bak 's/jboss\/keycloak\:7\.0\.1/ghcr\.io\/salsadigitalauorg\/rockpool\/keycloak\:7\.0\.1/g' Dockerfile
  sed -i .bak 's/${TINI_VERSION}\/tini/${TINI_VERSION}\/tini\-arm64/g' Dockerfile
  sed -i .bak 's/\/var\/cache\/yum/\/var\/cache\/yum \&\& ln -s \/usr\/bin\/python2 \/usr\/bin\/python/g' Dockerfile
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/keycloak:${LAGOON_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/keycloak:${LAGOON_VERSION}
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
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/broker-single:latest .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/broker-single:latest
  popd
}

function lagoon_broker () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/broker
  docker build  \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --build-arg IMAGE_REPO=${ROCKPOOL_IMAGES_REPO}/lagoon \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/broker:${LAGOON_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/broker:${LAGOON_VERSION}
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon_oc () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd images/oc
  sed -i .bak 's/GLIBC_VERSION=2\.28\-r0/GLIBC_VERSION=2\.30\-r0/g' Dockerfile
  sed -i .bak 's/sgerrand\/alpine-pkg-glibc/Rjerk\/alpine-pkg-glibc/g' Dockerfile
  sed -i .bak 's/${GLIBC_VERSION}\//${GLIBC_VERSION}\-arm64\//g' Dockerfile
  sed -i .bak 's/apk\ add\ glibc\-bin\.apk/apk\ add\ \-\-allow\-untrusted\ glibc\-bin\.apk/g' Dockerfile
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/oc .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/oc
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon_auto_idler () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/auto-idler
  docker build  \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --build-arg IMAGE_REPO=${ROCKPOOL_IMAGES_REPO}/lagoon \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/auto-idler:${LAGOON_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/auto-idler:${LAGOON_VERSION}
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon_storage_calculator () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd services/storage-calculator
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --build-arg IMAGE_REPO=${ROCKPOOL_IMAGES_REPO}/lagoon \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/storage-calculator:${LAGOON_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/storage-calculator:${LAGOON_VERSION}
  popd
  if [ ! "${fetched}" == "true" ]; then
    popd
  fi
}

function lagoon_docker_host () {
  fetched=$1
  if [ ! "${fetched}" == "true" ]; then
    lagoon_clone
  fi
  pushd images/docker-host
  docker build \
    --label "org.opencontainers.image.source=${ROCKPOOL_REPO}" \
    --tag ${ROCKPOOL_IMAGES_REPO}/lagoon/docker-host:${LAGOON_VERSION} .
  docker push ${ROCKPOOL_IMAGES_REPO}/lagoon/docker-host:${LAGOON_VERSION}
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
  lagoon_oc true
  lagoon_auto_idler true
  lagoon_storage_calculator true
  lagoon_docker_host true
  popd
}

# Build nfs-provisioner image.
function nfs_provisioner () {
  pushd ..
  docker buildx bake nfs-provisioner --push
  popd
}

target=${1:-all}

mkdir -p build
cd build

$target
