# target build source: https://github.com/markus-perl/ffmpeg-build-script/blob/v1.48/Dockerfile
ARG BASE_IMAGE=ubuntu:24.04@sha256:72297848456d5d37d1262630108ab308d3e9ec7ed1c3286a32fe09856619a782
FROM ${BASE_IMAGE} AS build

ARG FFMPEG_BUILD_SCRIPT_VERSION=1.48

ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update \
    && apt-get -y --no-install-recommends install \
        build-essential \
        curl \
        ca-certificates \
        libva-dev \
        python3 \
        python-is-python3 \
        ninja-build \
        meson \
    && apt-get clean; rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* \
    && update-ca-certificates

WORKDIR /app

# ADD doesn't cache when used from URL
RUN curl -sLO \
    https://raw.githubusercontent.com/markus-perl/ffmpeg-build-script/v${FFMPEG_BUILD_SCRIPT_VERSION}/build-ffmpeg && \
    chmod 755 ./build-ffmpeg && \
    SKIPINSTALL=yes ./build-ffmpeg \
        --build \
        --enable-gpl-and-non-free && \
    rm -rf packages && \
    find workspace -mindepth 1 -maxdepth 1 -type d ! -name 'bin' -exec rm -rf {} \; && \
    find workspace/bin -mindepth 1 -maxdepth 1 -type f ! -name 'ff*' -exec rm -f {} \;

FROM debian:trixie-20240110-slim as base

RUN apt-get update \
    && apt-get install -y \
        ca-certificates \
        mkvtoolnix \
        libva-drm2 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/workspace/bin/ff* /usr/bin/

# Check shared library
RUN ldd /usr/bin/ffmpeg && \
    ldd /usr/bin/ffprobe && \
    ldd /usr/bin/ffplay

FROM base as server
COPY ./dist/gearr-server /app/gearr-server

ENTRYPOINT ["/app/gearr-server"]

FROM base as worker
COPY ./dist/gearr-worker /app/gearr-worker

ENTRYPOINT ["/app/gearr-worker"]

FROM tentacule/pgstosrt as worker-pgs
COPY ./dist/gearr-worker /app/gearr-worker

ENTRYPOINT ["/app/gearr-worker"]
