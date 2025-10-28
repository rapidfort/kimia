#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/Lightstreamer/Docker.git \
  --dockerfile=7.3/jdk17/Dockerfile \
  --destination=lightstreamer \
  --no-push \
  -v