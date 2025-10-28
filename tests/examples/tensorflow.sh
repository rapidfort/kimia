#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
    --context=https://github.com/tensorflow/build.git \
    --context-sub-path=tensorflow_runtime_dockerfiles \
    --dockerfile=cpu.Dockerfile \
    --destination=10.228.96.114:5000/my-tensorflow \
    --no-push \
    -v
