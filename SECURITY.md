# Security Policy

LUBAN Code executes model-selected tools. Treat provider credentials, MCP servers, hooks, worktrees and shell access as security boundaries, not ordinary preferences.

## Supported version

The repository currently publishes source preview `v0.1.0` from `main`. There are no tagged releases or release binaries yet. Security fixes therefore land on `main`; no older branch has a support commitment.

## Reporting a vulnerability

Use GitHub's [private vulnerability-reporting form](https://github.com/agent-dance/luban/security/advisories/new). Do not put exploit details, credentials, private repository content or a harmful proof of concept in a public issue.

A useful first report includes:

- affected commit and operating system;
- the boundary crossed, such as permission, sandbox, credential, hook, MCP or provider transport;
- reproduction steps with secrets removed;
- expected and observed behavior;
- whether exploitation requires an untrusted repository, model response or remote server.

Please do not test against systems, accounts or endpoints you do not own.

## Current platform boundary

Linux sandboxing requires Bubblewrap. macOS uses `sandbox-exec`. Windows has no OS sandbox backend at present. Use `--force-sandbox-tools` when a run must fail rather than continue without a verified OS sandbox.

Local credentials are stored as plaintext JSON. Unix-like systems write them with mode `0600`; Windows currently has no equivalent ACL guarantee. They are not encrypted with an operating-system keychain. Never commit `.luban-code` credential material, `.env` files, API keys or OAuth tokens.
