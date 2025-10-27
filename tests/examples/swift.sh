#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/swiftlang/swift-docker.git \
  --dockerfile=nightly-5.10/ubuntu/22.04/Dockerfile \
  --destination=swift \
  --no-push \
  -v