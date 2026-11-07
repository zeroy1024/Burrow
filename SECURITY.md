# Security policy

[English](SECURITY.md) | [简体中文](SECURITY.zh.md)

## Reporting vulnerabilities

Do not publish exploit details for unfixed vulnerabilities, real credentials, private keys, databases, or raw audit logs in public Issues or PRs.

Submit a private report through **Security → Advisories → Report a vulnerability** in the GitHub repository. If that entry is not enabled, use the maintainer's publicly listed email in commit history to send an initial message without secrets. Confirm a private communication channel before providing details.

Include the affected version or commit, OS and architecture, required configuration, minimal reproduction steps, actual impact, and redacted evidence. Reproduce issues only in environments you own or are authorized to use.

## Fix scope

The project is in its 0.x stage. Security fixes first enter the current development branch and ship in subsequent releases. There is no commitment to backports or fixed response times. Remediation and disclosure are coordinated after reproduction.

## Data boundaries

Burrow can access servers, local files, and credentials authorized by the user. MCP access keys can receive explicitly granted SSH and file capabilities. Give keys only to trusted clients and restrict connections, capabilities, and local directory scope.

The database is not encrypted as a whole. Connection details, paths, MCP commands, and stdout/stderr may contain sensitive data. Automatic field redaction cannot guarantee removal of all secrets. Manually redact diagnostic material before sharing it. See [data and credentials](docs/storage.md) for storage and backup boundaries.
