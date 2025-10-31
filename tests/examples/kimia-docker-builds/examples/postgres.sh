#!/bin/bash
set -Eeuo pipefail
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/docker-library/postgres.git \
  --context-sub-path=18/alpine3.22 \
  --dockerfile=Dockerfile \
  --destination=postgres \
  --no-push \
  -v
