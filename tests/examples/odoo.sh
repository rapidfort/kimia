#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/odoo/docker.git \
  --context-sub-path=19.0 \
  --dockerfile=Dockerfile \
  --destination=odoo \
  --no-push \
  -v