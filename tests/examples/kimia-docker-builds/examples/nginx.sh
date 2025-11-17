#!/bin/bash
set -Eeuo pipefail
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v "$HOME/.docker/config.json":/home/kimia/.docker:ro \
  "$KIMIA_IMAGE" \
  --context=https://github.com/nginx/docker-nginx.git \
  --context-sub-path=mainline/alpine/ \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/nginx \
  --no-push \
  -v
