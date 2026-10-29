# Security Policy

Oxynote holds the pages that explain your systems, and it connects to your
databases and Prometheus servers. A security bug in it can reach well past
the app, so we'd like to hear about one early and in private.

## Reporting a Vulnerability

Report it through
[GitHub Security Advisories](https://github.com/oxynote/oxynote/security/advisories/new),
or email [security@oxynote.io](mailto:security@oxynote.io) if that doesn't
work for you. Please don't open a public issue, discussion or pull request
for it.

Include what you can:

- the release you tested, such as `v0.9.3`, or the commit and image tag if
  you built from source
- the steps to reproduce it
- what an attacker gains: whose data, and whether it crosses a sign-in or
  workspace boundary
- a fix or workaround, if you have one

Check that the issue reproduces before you send it. We close reports that
look like unverified output from a scanner or an AI tool.

## What to Expect

- We aim to reply within 10 business days.
- Once we confirm the issue, we tell you roughly when to expect a fix and
  keep you posted.
- The release notes credit you under the name or handle you choose. Tell
  us if you'd rather stay anonymous.
- Please wait for the release with the fix before you publish. If 90 days
  pass without one, go ahead.

Oxynote is open source and has no security budget, so we can't pay
bounties. We can fix the bug, credit you and say thank you.

## Supported Versions

Until Oxynote reaches 1.0, only the latest release gets security fixes. A
fix lands on `main` and ships in the next release, and a severe one gets a
patch release of its own. We don't backport fixes, because a 0.x minor
release may carry breaking changes anyway. If you found an issue in an
older release, check that it still reproduces in the latest one.

## Scope

Everything built from this repository is in scope. Some of it looks
alarming but works as intended:

- Core and auth-realtime have internal endpoints with no session check.
  The front door blocks them, and in the production image they listen
  only inside the container. Reaching one from outside is a finding.
- Webhook and OAuth callback endpoints have no session either. Each
  request is proven by the provider's signature or an encrypted state, so
  a forged request that passes that check is a finding.
- The dev stack under `docker/` is for local development, with example env
  files, default credentials and open service ports. The production image
  in `docker/prod/` is in scope.
- The production image shows its default admin login on the login page
  until the admin changes the password.
- A workspace's members can query the data sources connected to it.
  Reaching another workspace's data source, getting more access than you
  were granted, or making the server send requests to arbitrary internal
  hosts is a finding.
- Official images contain Sentry DSNs. A DSN only lets the image send
  crash reports, so it isn't a leaked secret.

These are out of scope:

- Vulnerabilities in third-party dependencies. Report them upstream, then
  open a normal issue or pull request here to update the dependency.
- Attacks that need a compromised host, a malicious operator, physical
  access or someone already privileged in the target workspace.
- Misconfiguration, such as exposing core without the front door, keeping
  the example secrets or the default password, or serving without TLS.
- Denial of service, load testing and automated scanning of anything that
  isn't yours.
- Social engineering, spam and email spoofing.

## Testing Responsibly

Test only against your own instance. The quickest one to start is the
`docker run` command in the [README](README.md#quick-start). Pull only as
much data as you need to show the issue, and never change or delete data
that belongs to someone else.
