# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| latest  | Yes                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email security@sandboxmatrix.dev with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will acknowledge receipt within 48 hours and aim to provide a fix within 7 days for critical issues.

## Security Measures

sandboxMatrix takes security seriously as a sandbox orchestrator:

- All sandbox isolation is enforced at the runtime level (Docker, Firecracker, gVisor)
- Network policies restrict sandbox-to-host communication by default
- Workspace mounts are read-only unless explicitly configured
- No secrets are stored in plaintext
