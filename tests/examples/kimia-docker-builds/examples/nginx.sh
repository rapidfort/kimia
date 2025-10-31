#!/bin/bash
set -Eeuo pipefail
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  "$KIMIA_IMAGE" \
  --context=https://github.com/nginx/docker-nginx.git \
  --context-sub-path=mainline/alpine/ \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/nginx \
  --no-push \
  -v
