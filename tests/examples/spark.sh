#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=https://github.com/apache/spark-docker.git \
  --context-sub-path=4.0.0/scala2.13-java21-ubuntu \
  --dockerfile=Dockerfile \
  --destination=10.228.96.114:5000/spark:4.0.0-scala2.13-java21-ubuntu \
  --no-push