#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:1.0.18 \
  --context=https://github.com/docker-library/mongo.git \
  --destination=10.228.96.114:5000/mongo \
  --context-sub-path=8.0/ \
  --dockerfile=Dockerfile \
  --no-push \
  -v
