#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/docker-library/golang.git \
  --context-sub-path=1.24/trixie \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/my-golang \
  --no-push