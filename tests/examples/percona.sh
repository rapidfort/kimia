#!/bin/bash
docker login
docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined ghcr.io/rapidfort/kimia:latest \
  --context=git://github.com/percona/percona-xtradb-cluster-operator.git \
  --dockerfile=build/Dockerfile \
  --destination=percona \
  --no-push \