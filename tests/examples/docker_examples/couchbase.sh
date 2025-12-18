#!/bin/bash
source test-env.sh
echo ${DOCKER_REGISTRY_PASSWORD} | docker login ${DOCKER_REGISTRY} -u ${DOCKER_REGISTRY_USERNAME} --password-stdin
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ${KIMIA_BUILDKIT_IMAGE} \
  --context=https://github.com/couchbase/docker.git \
  --context-sub-path=enterprise/couchbase-server/7.6.7 \
  --dockerfile=Dockerfile \
  --destination=${DESTINATION_REPO}/couchbase:${IMAGE_TAG} \
  --no-push \
  -v