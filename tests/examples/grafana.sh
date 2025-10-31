#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/grafana/grafana.git \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/grafana \
  --no-push \
  -v