#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/rust-lang/docker-rust.git \
  --dockerfile=stable/alpine3.21/Dockerfile \
  --destination=rust \
  --no-push \
  -v