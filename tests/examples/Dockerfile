FROM alpine@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412

ARG SOURCE_DATE_EPOCH

RUN apk add --no-cache \
    curl=8.14.1-r2 \
    ca-certificates=20250911-r0

WORKDIR /app

RUN echo "reproducible test" > file.txt

CMD ["sh"]
