#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/jenkinsci/docker.git \
  --context-sub-path="" \
  --dockerfile=debian/trixie/hotspot/Dockerfile \
  --destination=jenkins \
  --no-push