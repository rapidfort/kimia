#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/docker-library/python.git \
  --context-sub-path=3.14/alpine3.22 \
  --dockerfile=Dockerfile \
  --destination=python \
  --no-push \
  -v