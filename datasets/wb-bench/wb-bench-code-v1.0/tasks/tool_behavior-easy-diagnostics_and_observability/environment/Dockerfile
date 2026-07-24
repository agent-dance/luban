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

RUN bash -lc 'python -m pip install --upgrade pip && pc=$(python -c "import tomllib, pathlib; pkgs = tomllib.loads(pathlib.Path(\"poetry.lock\").read_text())[\"package\"]; print(next(p[\"version\"] for p in pkgs if p[\"name\"] == \"poetry-core\"))") && dv=$(python -c "import tomllib, pathlib; pkgs = tomllib.loads(pathlib.Path(\"poetry.lock\").read_text())[\"package\"]; print(next(p[\"version\"] for p in pkgs if p[\"name\"] == \"dulwich\"))") && python -m pip install --no-deps -e . && python -m pip install "poetry-core==$pc" "dulwich==$dv" "build>=1.2.1,<2" "cachecontrol[filecache]>=0.14,<0.15" "cleo>=2.1,<3" "fastjsonschema>=2.18,<3" "installer>=0.7,<0.8" "keyring>=25.1,<26" "packaging>=24,<27" "pkginfo>=1.12,<2" "platformdirs>=3,<5" "pyproject-hooks>=1,<2" "requests>=2.26,<3" "requests-toolbelt>=1,<2" "shellingham>=1.5,<2" "tomlkit>=0.11.4,<1" "trove-classifiers>=2022.5.19" "virtualenv>=20.26.6,<21" "findpython>=0.6.2,<0.8" "pbs-installer[download,install]>=2025.1.6,<2026" pytest pytest-mock pytest-randomly pytest-xdist deepdiff responses jaraco-classes httpretty "urllib3<2.3"'


# Initialize git repo for diff tracking
RUN git config --global user.email "dev@project" && \
    git config --global user.name "Developer" && \
    cd /workspace && \
    (git rev-parse --git-dir > /dev/null 2>&1 || (git init && git add -A && git commit -m "initial setup"))

WORKDIR /workspace