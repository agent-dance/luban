FROM python:3.12-slim

ARG NODE_VERSION=20.18.1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends git curl bash ca-certificates xz-utils procps && \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
        | tar xJ -C /usr/local --strip-components=1 && \
    rm -rf /var/lib/apt/lists/* /tmp/*
WORKDIR /workspace

# Scorer (single canonical copy, baked into image)
COPY scorer.py /scorer/scorer.py


# Extract the task workspace (baked into Docker context)
COPY workspace.tar.gz /tmp/workspace.tar.gz
RUN tar xzf /tmp/workspace.tar.gz -C /workspace && rm /tmp/workspace.tar.gz


# Install project dependencies if applicable

RUN python -c "from pathlib import Path; Path('sitecustomize.py').write_text('import collections\\nimport collections.abc\\nimport re\\nfor _name in (\\'Container\\', \\'Iterable\\', \\'Iterator\\', \\'Mapping\\', \\'MutableMapping\\', \\'Sequence\\', \\'Set\\'):\\n    if not hasattr(collections, _name) and hasattr(collections.abc, _name):\\n        setattr(collections, _name, getattr(collections.abc, _name))\\nif not hasattr(re, \\'PatternError\\'):\\n    re.PatternError = re.error\\n')" && python -m pip install --upgrade pip setuptools wheel pytest


# Initialize git repo for diff tracking
RUN git config --global user.email "dev@project" && \
    git config --global user.name "Developer" && \
    cd /workspace && \
    (git rev-parse --git-dir > /dev/null 2>&1 || (git init && git add -A && git commit -m "initial setup"))

WORKDIR /workspace