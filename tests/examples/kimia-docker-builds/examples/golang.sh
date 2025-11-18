#!/bin/bash
set -Eeuox pipefail

chmod 777 /root/.docker/config.json
ls -lla /root/.docker/config.json
cat /root/.docker/config.json

echo "docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v /root/.docker:/home/kimia/.docker \
  $KIMIA_IMAGE \
  --context=https://github.com/docker-library/golang.git \
  --context-sub-path=1.24/trixie \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/my-golang \
  -v"

sleep 3600

docker run --rm --cap-drop ALL --cap-add SETUID --cap-add SETGID --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v "/root/.docker:/home/kimia/.docker" \
  "$KIMIA_IMAGE" \
  --context=https://github.com/docker-library/golang.git \
  --context-sub-path=1.24/trixie \
  --dockerfile=Dockerfile \
  --destination=harbor.rfinnovate.rapidfort.io/kimia-e2e/my-golang \
  -v
