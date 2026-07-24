FROM python:3.12-slim

ARG NODE_VERSION=20.18.1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends git curl bash ca-certificates xz-utils procps && \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
        | tar xJ -C /usr/local --strip-components=1 && \
    rm -rf /var/lib/apt/lists/* /tmp/*
WORKDIR /workspace

COPY workspace.tar.gz /tmp/workspace.tar.gz
RUN tar xzf /tmp/workspace.tar.gz -C /workspace && rm /tmp/workspace.tar.gz

RUN git config --global user.email "dev@project" && \
    git config --global user.name "Developer" && \
    cd /workspace && \
    (git rev-parse --git-dir > /dev/null 2>&1 || (git init && git add -A && git commit -m "initial setup"))

WORKDIR /workspace
