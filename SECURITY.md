# Security Policy

Oxynote holds operational knowledge — runbooks, credentials-adjacent
API documentation, and live connections to production databases and
metrics endpoints. We take reports about it seriously, and we would
rather hear about a problem early and awkwardly than late and
publicly.

## Supported versions

Oxynote follows [semantic versioning](https://semver.org) and ships
tagged releases (`v0.0.1`, `v0.1.0`, and so on).

While Oxynote is pre-1.0, **only the most recent release is
supported**. Security fixes land on `main` and ship in the next
release; a severe issue gets its own patch release rather than waiting
for the next scheduled one. We do not backport fixes to earlier minor
versions — during 0.x a minor bump may carry breaking changes, so the
upgrade path is forward, not sideways.

If you report an issue against an older release, please confirm it
still reproduces on the latest one. We will look at reports that only
affect an already-superseded version, but we will fix them by telling
you to upgrade.

This window widens once Oxynote reaches 1.0.

## Reporting a vulnerability

**Please do not open a public issue, discussion, or pull request for a
security problem.**

Report privately through
[GitHub Security Advisories](https://github.com/oxynote/oxynote/security/advisories/new).
If that does not work for you, email
[security@oxynote.io](mailto:security@oxynote.io).

Please include as much of this as you can:

- The release you reproduced against (`v0.0.1`), or the commit SHA
  and image tag if you build from source
- Steps to reproduce
- What an attacker gains: whose data, which organization, and whether
  it crosses an authentication or organization boundary
- Any patch or mitigation you have in mind

Verify that the issue actually reproduces before you report it. We
close reports that appear to be unverified machine-generated output.

## What to expect

- We aim to acknowledge a report within **5 business days**.
- If we confirm it, we will tell you roughly when we expect a fix and
  keep you updated as it moves.
- We will credit you in the fix's release notes under whatever name or
  handle you give us. Tell us if you would rather stay anonymous.
- We ask that you hold off on publishing until a release containing
  the fix is out. If 90 days pass without one, publish — that is on
  us, not you.

Oxynote is an open source project without a security budget. We cannot
pay bounties. We can fix the bug, credit you properly, and say thank
you.

## Scope

In scope: everything built from this repository.

Some things about Oxynote's architecture look alarming and are
intentional. Please read this before reporting them:

**Some sessionless surfaces are sessionless by design.** Core exposes
service-to-service endpoints that the front-door Caddyfile blocks, and
webhook and OAuth callback endpoints that authenticate by provider
signature or encrypted state rather than by session. That an endpoint
has no session check is not itself a finding. Reaching a blocked
endpoint through the front door, or forging a request that passes
signature validation, is.

**The dev stack is not hardened and is not meant to be.** Everything
under `docker/` — the committed example env files, the default
credentials, and the directly exposed service ports — exists for local
development. Findings that depend on running the dev compose stack in
a hostile environment are out of scope.

**Data sources connect where the operator points them.** That an
authorized user can query a database an admin connected is the
feature. Escaping that boundary is not: reaching another
organization's data source, exceeding the granted scope, or coercing
the server into requests against arbitrary internal hosts.

Out of scope:

- Vulnerabilities in third-party dependencies. Report those upstream —
  and please do open a normal public issue or PR here to bump the
  dependency.
- Anything requiring a compromised host, a malicious operator, physical
  access, or an actor already privileged in the target organization.
- Operator misconfiguration: exposing core directly instead of through
  Caddy, running with the example secrets, disabling TLS.
- Denial of service, volumetric testing, and automated scanning against
  infrastructure that is not yours.
- Social engineering, spam, and email spoofing.

## Testing responsibly

Test against your own instance. Do not test against anyone else's
deployment, do not pull more data than you need to demonstrate the
issue, and do not modify or delete data belonging to other people.
