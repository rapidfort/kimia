#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:1.0.18 \
  --context=https://github.com/nodejs/docker-node.git \
  --context-sub-path=24/bullseye/ \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/mynode \
  --no-push \
  -v
