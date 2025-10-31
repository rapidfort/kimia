#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/pytorch/pytorch.git \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/my-pytorch \
  --no-push