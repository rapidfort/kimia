#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/alpinelinux/docker-alpine.git \
  --context-sub-path=/aarch64/ \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/googlecloudsdk \
  --no-push \
  -v
