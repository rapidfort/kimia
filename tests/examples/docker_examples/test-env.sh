#Name: test-env.sh
#/bin/bash

KIMIA_BUILDAH_IMAGE=${KIMIA_BUILDAH_IMAGE:-"ghcr.io/rapidfort/kimia-bud:latest"}
KIMIA_BUILDKIT_IMAGE=${KIMIA_BUILDKIT_IMAGE:-"ghcr.io/rapidfort/kimia:latest"}
DESTINATION_REPO=${DESTINATION_REPO:-"docker.io/kimia-e2e"}
DOCKER_REGISTRY_USERNAME="${DOCKER_REGISTRY_USERNAME:-"testusername"}" # set env var for overriding DOCKER_REGISTRY_USERNAME
DOCKER_REGISTRY="${DOCKER_REGISTRY:-"docker.io"}"
DOCKER_REGISTRY_PASSWORD="${DOCKER_REGISTRY_PASSWORD:-"dummypassword"}" # set env var for overriding DOCKER_REGISTRY_USERNAME
IMAGE_TAG="${IMAGE_TAG:-"latest"}"
KIMIA_COMMON_ARGS="${KIMIA_COMMON_ARGS:-""}"
