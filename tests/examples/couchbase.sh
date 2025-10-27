#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/smithy:1.0.10 \
  --context=https://github.com/couchbase/docker.git \
  --context-sub-path=enterprise/couchbase-server/7.6.7 \
  --dockerfile=Dockerfile \
  --destination=couchbase \
  --no-push \
  -v