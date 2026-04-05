# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.3.x   | ✅ Current release |
| < 0.3   | ❌ No longer supported |

## Reporting a Vulnerability

If you discover a security vulnerability in MAGI, **please do not open a public issue.**

Instead, report it privately:

1. **Email**: Send details to the repository owner via GitHub's private vulnerability reporting
2. **GitHub**: Use the [Security Advisories](https://github.com/j33pguy/magi/security/advisories) tab to report privately

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if you have one)

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Assessment**: Within 1 week
- **Fix**: Depends on severity, but critical issues are prioritized immediately

## Security Architecture

MAGI handles sensitive data (AI agent memories, decisions, conversations). Key security measures:

### Authentication
- Bearer token authentication on all API endpoints
- Machine token enrollment for magi-sync clients
- Token-based access scoping (owner vs machine credentials)
- Read-only mode enforced when no auth is configured (write methods blocked, not just logged)

### Data Protection
- All data stored locally — no external cloud dependencies
- Optional Vault integration for secret management
- Privacy controls in magi-sync (allowlist mode, secret redaction)
- Visibility scoping on memories (internal, private, public)

### Network
- gRPC and HTTP servers bind to configurable addresses
- TLS termination expected at reverse proxy (Traefik, nginx, etc.)
- No default external network calls except optional Turso sync

### CI/CD Security
- Self-hosted runners restricted to trusted actors only (`github.actor` allowlist)
- `pull_request_target` trigger ensures workflow definitions come from `main`, not PR branches
- PRs that modify `.github/workflows/` are automatically rejected in CI
- Gitleaks secret scanning runs on every PR (GitHub-hosted runner, not self-hosted)
- Pre-commit hooks available via `.pre-commit-config.yaml` (gitleaks)

## Scope

The following are **in scope** for security reports:

- Authentication bypass
- Unauthorized data access
- Injection vulnerabilities (SQL, command)
- Information disclosure
- Denial of service

The following are **out of scope**:

- Issues requiring physical access to the server
- Social engineering
- Vulnerabilities in dependencies (report upstream, but let us know)
- Issues in development/test configurations
