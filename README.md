<div align="center">

# Oxynote

**Your system, with subtitles.**

A self-hosted workspace that puts a live metric chart next to the paragraph
explaining it, gives your internal APIs Stripe-style docs, and lets a change
go through a draft, a diff and an approval. From the authors of
[ttlcache](https://github.com/jellydator/ttlcache).

[![License](https://img.shields.io/github/license/oxynote/oxynote)](LICENSE)
[![Release](https://img.shields.io/github/v/release/oxynote/oxynote)](https://github.com/oxynote/oxynote/releases)
[![CI](https://img.shields.io/github/check-runs/oxynote/oxynote/main?label=CI)](https://github.com/oxynote/oxynote/actions)

</div>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/hero-dark.png">
  <img src="docs/images/hero-light.png" alt="The Shipments page of a Booking API in Oxynote: a Split Documentation block with the response fields on the left and, on the right, a curl request under a GET /v1/shipments/{id} title and a live p99 latency chart with Degraded and Heavy delay thresholds. Tags and maintainers in the page header, the page tree and tag list in the sidebar.">
</picture>


## Why Oxynote?

The place Grafana gives you to explain a panel is its description field, a
tooltip behind an info icon that Grafana's own docs tell you to keep short.
Keep the explanation in Notion instead, and the chart can only appear there
as an embedded Grafana panel, which renders only for someone already signed
in to Grafana, and not at all on Grafana Cloud. Each tool keeps its own
history, so nobody ever reviews the chart and its explanation as one change.
In Oxynote the chart runs its own query inside the page, the explanation is
whatever you type under it, and a draft branch carries both through one diff
and one approval.

<details>
<summary>The long version is a bit of a rant, but it explains the <em>personal</em> why</summary>

For a long time, the most reliable documentation at my company was the
Slack chat I had with myself. How it got that way starts with
dashboards. We had a lot of them: Grafana, Sentry, for some reason a bit
of Datadog, and a few custom self-hosted HTML pages with charts on them.
On top of that, I was encouraged to run my own health checks for the
deployments I owned (why that could not live in Grafana or anything like
it, nobody could tell me). Those deployments were services pulling
financial market data over websockets. Under heavy volume they leaked
memory and got OOM killed. There was a bug in there somewhere that we
were chasing and could not find for a while, and in the meantime every
kill meant missed events, and missed events meant running backfill
scripts by hand. None of our dashboards showed any of that. So: cron, curl
against each service's status endpoint, and the result posted to me
privately on Slack. Not convenient at all.

Then there was everything else. Internal wikis still describing
deployment procedures for trial products that had ended. Secret vaults.
Server management commands and curl requests. None of it lived in one
place, and access was its own problem: each of these had a different
owner who could grant it, so a lot of my time went into finding out who
that was and then finding them. Since we talked on Slack, that is where
I asked for all of it, and that is where people answered, sometimes with
plain credentials (the horror). To not lose any of it, I forwarded those
messages to the chat with myself, which is how that chat became my
source of truth for a long time: links to the specific Grafana
dashboards that mattered out of the many we had, example curl requests
against other teams' services so I could match their responses when
integrating (neither the requests nor the responses were documented
anywhere), and server IPs with notes on what was deployed where.

Naturally, I did try to fix it. I wrote Notion docs and linked the
Grafana charts from them, and for a while that was fine. Then they
drifted apart. One doc explained the labels on our ingestion-rate chart,
market, region and so on, but new labels kept being added, nobody
documented them, half were not even used in the chart, and each one cost
Prometheus another time series. Keeping a doc and a chart in sync meant
links on both sides, in Notion and in Grafana, which was more work than
I wanted, plus switching browser tabs just to check that the explanation
and the chart still meant the same thing. And that was only
observability. Beyond it, admin actions were specific curl requests, and
feature flags lived in a service disconnected from everything else. All
of it deserved a proper review flow, like on GitHub, with diffs that work
for text, metric charts and other visual structures. Instead it had
none.

David ([@davseby](https://github.com/davseby)), who started Oxynote with me
([@swithek](https://github.com/swithek)), was the one who had to work
through a system and its documents the first time he was on call, at a
different company. He had years of Grafana and internal docs behind him and
expected it to be routine. What he got was Datadog, which he had never used,
and a wildly different Notion structure. Then, on that first shift, the
services that enrich image metadata failed. Search over user uploads turned
inaccurate, and from there it cascaded through the product. Monitors fired,
but none of them linked anywhere or said which service had started failing,
which made for a nerve-racking night of tying things together by hand. The
metric showing one service overloaded and unresponsive existed the whole
time. Datadog's UI is convoluted, though, and nothing explained what any
chart tracked, so David never found it. An AI agent connected to Datadog's
MCP server did.

What both of us wanted was the graph, the explanation, the command and
the flag on one page, edited together, reviewed like code, and readable
by a tired person at night or by an agent. Every tool we had was built
to be great at one of those things and to never look sideways at the
others. That gap is what Oxynote is for. It starts with the pieces we
needed most: live Notion-like docs, metric charts, review flows, and
Stripe-like split docs for real-time systems. The other pieces of the
puzzle are coming.

</details>

## Quick start

Oxynote is one container. This is all it takes:

```sh
docker run -d --name oxynote \
  -p 8080:8080 \
  -v oxynote_data:/oxynote/data \
  ghcr.io/oxynote/oxynote:latest
```

Open http://localhost:8080 and log in with `admin@example.com` and
`oxynote-admin-1234`.

To run from source, see [CONTRIBUTING.md](CONTRIBUTING.md).

## What is in the box

### Metric grids

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/metric-grid-dark.png">
  <img src="docs/images/metric-grid-light.png" alt="The Release health page in Oxynote: a paragraph and a section called The Friday rule, then a metric grid with a Friday deploys bar chart, a hotfixes per release line chart with a Needs attention threshold, and deploy confidence gauges.">
</picture>

A `Live Metrics` block shows a time series, a bar chart or a gauge, built
from one or more Prometheus or SQL queries, with PostgreSQL, MySQL and
MariaDB supported on the SQL side. A metric grid is several of those blocks
side by side. In Grafana that would be a dashboard. In Oxynote it is a
dashboard inside a page, which means the explanation goes around it: what a
chart shows, why the threshold sits where it does, what to do when it is
crossed, all on the same page as the charts.

### Reviews

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/review-dark.png">
  <img src="docs/images/review-light.png" alt="A draft of the quote-engine page under review in Oxynote with Show Changes on: removed text in red and added text in green across a paragraph, the endpoint title of a code block, parameter names and the JSON response. Maintainers, a reviewer and an Approve Draft button in the header.">
</picture>

Any page can be made reviewable, and from then on every change goes
into a draft. The diff understands blocks, not just text: a renamed
endpoint, a renamed response field and a reworded sentence show up in one
view, red and green, in place. Reviewers approve, the draft merges into
main, and main can be protected so nobody edits it directly.

### Collaboration

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/collaboration-dark.png">
  <img src="docs/images/collaboration-light.png" alt="Two people editing the Quote lifecycle page in Oxynote at the same time: one has a chart block selected, the other is typing in the source of a Mermaid state diagram. A comment thread is open on the words 8s timeout in that source, with a reply, a reply box and a Resolve button. The inbox in the sidebar shows one unread item.">
</picture>

Pages are edited live, so you see who is on the page, which block they
are holding and where they are typing, and nobody overwrites anybody.
Comments attach to a word, to a line in a diagram's source or to a whole
chart, carry replies and get resolved when the change lands. Every
mention and reply arrives in your inbox.

### The rest

- Blocks: headings, lists, checklists, quotes, titled code blocks,
  callouts, images, files, Figma embeds, Mermaid diagrams.
- Freshness hooks: a block or page tracks the GitHub files, container
  image, web page or date it depends on, and its maintainers get a
  reminder when that changes.
- A page tree with sub pages, tags, full-text search, keyboard shortcuts
  for everything, light and dark themes.
- Workspaces with invitations, GitHub and Slack apps, and social login
  through GitHub, Google or Slack.
- Rubber Duck, the built-in assistant, edits pages and runs the same
  queries the blocks run, with the model of your choice: Anthropic,
  OpenAI, Google, OpenRouter, or a local one through Ollama. MCP clients
  such as Claude Code and Codex get the same tools over OAuth.

## Simple on purpose

A few honest words about who this is for.

Oxynote is for you if:

- you explain systems to other tech people and want the metric chart and
  the explanation on one page
- you want Stripe-style docs for internal services without a doc
  generator
- you want changes to a page to go through a draft, a diff and an
  approval, like a pull request
- you want one page that your team and your agents both read
- you self-host

Probably not for you if:

- you want a hosted service. There is none for now
- you want a wiki for the whole company. This is for tech people and the
  systems they run
- you want every option under the sun. Oxynote is opinionated, ships good
  defaults and keeps the option count low

## FAQ

**Does it replace Grafana?** That is the goal. Today it covers metric
charts and dashboard-style grids from Prometheus and SQL, with the
explanation written next to them. Logs and alerting are not there yet, but we
are working on them.

**Why the name?** Internally we first called this "breathing docs".
Breathing needs oxygen, so oxy became the prefix, and note followed.
