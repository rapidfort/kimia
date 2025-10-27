#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/rust-lang/docker-rust.git \
  --dockerfile=stable/alpine3.21/Dockerfile \
  --destination=rust \
  --no-push \
  -v \
  > kimia_rust.log 2>&1

docker run --rm --security-opt seccomp=unconfined --security-opt apparmor=unconfined gcr.io/kaniko-project/executor:latest \
  --context=git://github.com/rust-lang/docker-rust.git \
  --dockerfile=stable/alpine3.21/Dockerfile \
  --destination=rust \
  --no-push \
  > kaniko_rust.log 2>&1