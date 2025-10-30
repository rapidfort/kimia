#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/docker-library/ruby.git \
  --dockerfile=3.5-rc/trixie/Dockerfile \
  --destination=ruby \
  --no-push \
  -v