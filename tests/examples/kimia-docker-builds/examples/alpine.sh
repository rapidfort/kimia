#!/bin/bash
set -Eeuo pipefail
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:1.0.20 \
  --context=git://github.com/alpinelinux/docker-alpine.git \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/my-alpine \
  --no-push
 > kimia-bud_alpine.log 2>&1