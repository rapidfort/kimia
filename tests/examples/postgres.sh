#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/docker-library/postgres.git \
  --context-sub-path=18/alpine3.22 \
  --dockerfile=Dockerfile \
  --destination=postgres \
  --no-push \
  -v
