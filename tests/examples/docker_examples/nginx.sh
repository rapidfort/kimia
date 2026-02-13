#!/bin/bash
source test-env.sh
echo ${DOCKER_REGISTRY_PASSWORD} | docker login ${DOCKER_REGISTRY} -u ${DOCKER_REGISTRY_USERNAME} --password-stdin
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  ${KIMIA_BUIDKIT_IMAGE} \
  --context=https://github.com/nginx/docker-nginx.git \
  --context-sub-path=mainline/alpine/ \
  --dockerfile=Dockerfile \
  --destination=${DESTINATION_REPO}/nginx:${IMAGE_TAG} \
  --no-push \
  -v
