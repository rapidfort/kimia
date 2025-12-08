#!/bin/bash
source test-env.sh
echo ${DOCKER_REGISTRY_PASSWORD} | docker login ${DOCKER_REGISTRY} -u ${DOCKER_REGISTRY_USERNAME} --password-stdin
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  ${KIMIA_BUILDKIT_IMAGE} \
  --context=https://github.com/docker-library/mysql.git \
  --context-sub-path=8.0/ \
  --dockerfile=Dockerfile.debian \
  --destination=${DESTINATION_REPO}/mysql:${IMAGE_TAG} \
  --no-push \
  -v
