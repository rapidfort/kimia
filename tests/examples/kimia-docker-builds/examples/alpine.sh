#!/bin/bash
set -Eeuo pipefail
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v $HOME/.docker:/kimia/.docker:ro \
  "$KIMIA_IMAGE" \
  --context=git://github.com/alpinelinux/docker-alpine.git \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/alpine \
  -v