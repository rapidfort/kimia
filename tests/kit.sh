#!/bin/bash
docker run --rm -it \
    --entrypoint= \
	--security-opt seccomp=unconfined \
	--security-opt apparmor=unconfined \
	--cap-add SETUID \
	--cap-add SETGID \
	${RF_APP_HOST}:5000/rapidfort/kimia bash
