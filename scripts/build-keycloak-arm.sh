#!/bin/bash

set -ex

[ ! -d "build/keycloak-containers" ] && git clone https://github.com/keycloak/keycloak-containers.git build/keycloak-containers
cd build/keycloak-containers && git checkout -- . && git clean -fd . && git checkout 7.0.1 && cd server
docker build -t ghcr.io/yusufhm/rockpool/keycloak-arm:7.0.1 .
docker push ghcr.io/yusufhm/rockpool/keycloak-arm:7.0.1

cd ../../
[ ! -d "lagoon" ] && git clone https://github.com/uselagoon/lagoon.git
cd lagoon && git checkout -- . && git clean -fd . && git checkout v2.5.0 && cd services/keycloak
sed -i .bak 's/jboss\/keycloak\:7\.0\.1/rockpool\/keycloak\-arm\:7\.0\.1/g' Dockerfile
sed -i .bak 's/${TINI_VERSION}\/tini/${TINI_VERSION}\/tini\-arm64/g' Dockerfile
sed -i .bak 's/\/var\/cache\/yum/\/var\/cache\/yum \&\& ln -s \/usr\/bin\/python2 \/usr\/bin\/python/g' Dockerfile
docker build -t ghcr.io/yusufhm/rockpool/lagoon-keycloak-arm:v2.5.0 .
docker push ghcr.io/yusufhm/rockpool/lagoon-keycloak-arm:v2.5.0

cd ../broker-single
docker build -t ghcr.io/yusufhm/rockpool/lagoon-broker-single-arm:v2.5.0 .
docker push ghcr.io/yusufhm/rockpool/lagoon-broker-single-arm:v2.5.0

cd ../broker
sed -i .bak 's/${IMAGE_REPO\:\-lagoon}\/broker-single/ghcr.io\/yusufhm\/rockpool\/lagoon-broker-single-arm\:v2\.5\.0/g' Dockerfile
docker build -t ghcr.io/yusufhm/rockpool/lagoon-broker:v2.5.0 .
docker push ghcr.io/yusufhm/rockpool/lagoon-broker:v2.5.0
