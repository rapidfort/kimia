#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/docker-library/php.git \
  --context-sub-path=8.4/trixie/apache \
  --dockerfile=Dockerfile \
  --destination=php \
  --no-push \
  -v