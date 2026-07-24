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

RUN python -m pip install --upgrade pip setuptools wheel && SETUPTOOLS_SCM_PRETEND_VERSION=2.9.0 python -m pip install -e . && python -m pip install pytest pytest-cov hypothesis freezegun attrs mock && python -c "from pathlib import Path; Path('pytest.ini').write_text('[pytest]\nfilterwarnings =\n    ignore::DeprecationWarning\n    ignore::PendingDeprecationWarning\n')" && python -c "import sysconfig; from pathlib import Path; p = Path(sysconfig.get_paths()['purelib']) / 'sitecustomize.py'; p.write_text(\"import warnings\\nwarnings.filterwarnings('ignore', category=DeprecationWarning)\\nwarnings.filterwarnings('ignore', category=PendingDeprecationWarning)\\n\")" && pip install pytest


# Initialize git repo for diff tracking
RUN git config --global user.email "dev@project" && \
    git config --global user.name "Developer" && \
    cd /workspace && \
    (git rev-parse --git-dir > /dev/null 2>&1 || (git init && git add -A && git commit -m "initial setup"))

WORKDIR /workspace