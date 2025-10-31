#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/docker-library/httpd.git \
  --context-sub-path=2.4/alpine \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/my-httpd \
  --no-push