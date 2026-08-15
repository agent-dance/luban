# Security Policy

## Supported versions

Until LUBAN Code reaches 1.0, security fixes are released for the latest published minor version only. Users should upgrade to the latest GitHub Release before reporting an issue that may already be fixed.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| Older releases | No |
| Unreleased `main` | Best effort |

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub's private vulnerability reporting flow for this repository:

1. Open the repository's **Security** tab.
2. Choose **Advisories** and **Report a vulnerability**.
3. Include affected versions, impact, reproduction steps, and any proposed mitigation.

If the private reporting option is unavailable, open a public issue containing only a request for a private security contact; do not include exploit details, credentials, private source, or user data.

Please allow the maintainers a reasonable period to investigate and coordinate a fix before disclosure. We will acknowledge a valid private report, assess impact, and communicate remediation and disclosure status through the advisory.

## Scope

Reports are especially useful for vulnerabilities involving:

- command execution or sandbox/permission bypass;
- unauthorized filesystem access;
- credential, prompt, source-code, or session-data disclosure;
- malicious repository content causing execution without the documented approval boundary;
- release artifact, installer, update, checksum, or provenance compromise.

API provider availability, model behavior without a security boundary impact, and exposed credentials that do not belong to the project are generally outside scope.

## Release verification

Official releases are published from `agent-dance/luban`. Verify downloaded artifacts using `checksums.txt` and GitHub artifact attestations as described in the [installation guide](docs/installation.md#校验发布资产).
