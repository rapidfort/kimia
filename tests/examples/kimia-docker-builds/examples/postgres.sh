#!/bin/bash
set -Eeuo pipefail
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v "/root/.docker/config.json:/home/kimia/.docker/config.json:ro" \
  "$KIMIA_IMAGE" \
  --context=https://github.com/docker-library/postgres.git \
  --context-sub-path=18/alpine3.22 \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/postgres \
  -v
