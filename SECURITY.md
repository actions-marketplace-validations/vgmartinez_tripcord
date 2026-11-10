# Security

Tripcord has no hosted service.

## Reporting a vulnerability

Report vulnerabilities in Tripcord through GitHub private vulnerability
reporting on this repository: open the
[Security tab](https://github.com/vgmartinez/tripcord/security) and
choose **Report a vulnerability**, or go straight to
[a new advisory](https://github.com/vgmartinez/tripcord/security/advisories/new).
That opens a private Security Advisory that only you and the
maintainers can see.

Do not open a public issue, pull request, or discussion for a security
report, and do not post details anywhere public. If the
**Report a vulnerability** button is missing, open an issue that asks
for a private reporting channel and contains no details of the problem.

Use the same channel to privately report malicious agent configurations
found in the wild, before a detection rule is published.

## Supported versions

Security fixes go to the latest release and to `main`. A fix ships in a
new release, and the advisory names the first fixed version.

## Verifying a release

Each release archive is listed in `SHA256SUMS` and has a build
provenance attestation. See
[Installation](docs/installation.md#from-a-release) for
`sha256sum -c SHA256SUMS` and
`gh attestation verify <file> --repo vgmartinez/tripcord`.
