#!/bin/bash
set -Eeuox pipefail

ls -lla /root/.docker/config.json
cat /root/.docker/config.json

docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
 -e DOCKER_CONFIG=/tmo/work/.docker/config.json \
  -v "/root/.docker/config.json:/tmp/work/.docker/config.json" \
  "$KIMIA_IMAGE" \
  --context=https://github.com/docker-library/golang.git \
  --context-sub-path=1.24/trixie \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/my-golang \
  -v
