# Security Policy

## Reporting a Vulnerability

If you discover a security issue in this provider, please **do not** open
a public GitHub issue. Instead, report it privately:

- Use GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability) on this repository, or
- Email the maintainers at the contact listed in [CODEOWNERS](./CODEOWNERS).

Please include:

- A description of the issue and its impact.
- Steps to reproduce.
- Provider version, Terraform version, and OS where applicable.
- Any proof-of-concept code or configuration.

We aim to acknowledge reports within 5 business days and to issue a fix
or mitigation within 30 days for high-severity issues. Lower-severity
issues will be addressed in the next scheduled release.

## Scope

This policy covers the code in this repository. Vulnerabilities in the
upstream [Datadog Feature Flags API](https://docs.datadoghq.com/api/latest/feature-flags/)
should be reported directly to Datadog.

## Disclosure

We coordinate disclosure with reporters. Public advisories will be
published via [GitHub Security Advisories](https://docs.github.com/en/code-security/security-advisories)
once a fix is available.
