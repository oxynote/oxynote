# Oxynote

**Your system, with subtitles.**

[![License](https://img.shields.io/github/license/oxynote/oxynote)](LICENSE)
[![Release](https://img.shields.io/github/v/release/oxynote/oxynote)](https://github.com/oxynote/oxynote/releases)
[![CI](https://img.shields.io/github/check-runs/oxynote/oxynote/main?label=CI)](https://github.com/oxynote/oxynote/actions)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/hero-dark.png">
  <img src="docs/images/hero-light.png" alt="The Shipments page of a Booking API in Oxynote: a Split Documentation block with the response fields on the left and, on the right, a curl request under a GET /v1/shipments/{id} title and a live p99 latency chart with Degraded and Heavy delay thresholds. Tags and maintainers in the page header, the page tree and tag list in the sidebar.">
</picture>


## Why Oxynote?

Running software comes with a lot of screens. Grafana or Datadog draws
the charts of your metrics, Notion or Confluence holds the pages that are
supposed to explain them, feature flags sit in yet another service, and
admin actions live in a folder of scripts. None of these products is bad
at its own job. The trouble starts the moment you need two of them at
once, which is every time something breaks.

Each of them does a little of the others' job, and you can tell it was an
afterthought. Grafana lets you describe a panel, but the text goes into a
tooltip behind an info icon, and Grafana's own docs suggest keeping it
short. Notion takes a Grafana link, and a link is what you get: an embedded
panel only renders for someone already signed in to Grafana, and not at all
on Grafana Cloud. Both keep a history of their own side, tracked separately,
so you can see that the chart changed and that the page changed, never that
they changed together, and nobody had to properly review or approve either.

So every time you learn something about a chart, you face the same
small decision: does this go into Grafana, into Notion, or into the Slack
thread where you are already typing? Usually the thread wins, and the
knowledge stays there. Oxynote removes the decision. The chart is a block
in the page, the explanation is the paragraph next to it, and there is
nowhere else for it to go. If the page matters, a change goes through a
review like a pull request does: a draft branch, a diff, an approval,
then a merge.

<details>
<summary>The long version is a bit of a rant, but it explains the personal <em>why</em></summary>

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

## What Oxynote is

Think of Oxynote as a Notion-style workspace that knows what a running
system is. A page is still a page: you type, others see it as you type,
blocks move around. The difference is what a block can be. One block is a
chart that runs a Prometheus query, or a SQL query on PostgreSQL, MySQL or
MariaDB, every time the page opens. The paragraph above it says what the
chart means. Because the chart is part of the page rather than a link to
somewhere else, there is no link to maintain and no second place to check.

The same idea carries into API docs. Stripe made the two-column reference
the standard, parameters on the left, request and response on the right,
and public doc generators copied it years ago. Internal tools never did,
which is why the docs for your own services end up as a wiki table. In
Oxynote that layout is a block called *Split Documentation*, and the right
column can hold code or a live chart, so the docs for a service and the
health of that service share a page.

When a page matters, you can put it under review. Changes go into a
draft, the diff shows text and charts changing side by side, reviewers
approve, and the draft merges into main. Pages that do not need that stay
plain.

And the reader is not always a person. Rubber Duck, the built-in
assistant, edits pages and runs the same queries the blocks run. You pick
its model: Anthropic, OpenAI, Google, OpenRouter, or a local one through
Ollama. MCP clients such as Claude Code, Codex and many others get the
same tools over OAuth, with whatever model they already run. People and
agents edit the same pages, and the pages stay designed for people.

## Quick start

Oxynote runs as one container next to a PostgreSQL database you already
have. It needs to know two things: the address people will open in the
browser, and how to reach that database.

```console
$ docker run -d --name oxynote \
  -p 8080:8080 \
  -e OXYNOTE_PUBLIC_URL=http://localhost:8080 \
  -e OXYNOTE_DB_DSN='postgresql://oxynote:change-me@postgres.example.com/oxynote?sslmode=disable' \
  -v oxynote_data:/oxynote/data \
  ghcr.io/oxynote/oxynote:latest
```

Open the address and sign up. Until you configure an email sender, the
verification link shows up in the container's logs instead of your inbox.
Keep the data volume: it holds your uploads and the secrets that keep
everyone signed in and every data source connected. For a real domain, put a
TLS-terminating proxy in front and set the public URL to the `https://`
address.

<details>
<summary>Docker Compose with PostgreSQL, Meilisearch for search and Mailpit for mail</summary>

```yaml
name: oxynote

services:
  oxynote:
    image: ghcr.io/oxynote/oxynote:latest
    ports:
      - "8080:8080"
    environment:
      - OXYNOTE_PUBLIC_URL=http://localhost:8080
      - OXYNOTE_DB_DSN=postgresql://oxynote:change-me-db@postgres/oxynote?sslmode=disable
      - OXYNOTE_MEILISEARCH_URL=http://meilisearch:7700
      - OXYNOTE_MEILISEARCH_MASTER_KEY=change-me-meili-key
      # point these at a real relay to have mail delivered; here they go
      # to the mailpit service below.
      - OXYNOTE_SMTP_DSN=smtp://mailpit:1025?tls=none
      - OXYNOTE_EMAIL_FROM_ADDRESS=Oxynote <team@example.com>
    volumes:
      - oxynote_data:/oxynote/data
    depends_on:
      postgres:
        condition: service_healthy
      meilisearch:
        condition: service_healthy
      mailpit:
        condition: service_started
    # the image shuts its services down in order, flushing open documents
    # last; docker's default 10s grace would kill that mid-flush.
    stop_grace_period: 60s
    restart: unless-stopped

  postgres:
    image: postgres:18.6-alpine
    environment:
      - POSTGRES_USER=oxynote
      - POSTGRES_PASSWORD=change-me-db
      - POSTGRES_DB=oxynote
    volumes:
      - postgres_data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U oxynote -d oxynote"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  # full-text search.
  meilisearch:
    image: getmeili/meilisearch:v1.53.1
    environment:
      - MEILI_ENV=production
      - MEILI_MASTER_KEY=change-me-meili-key
      - MEILI_NO_ANALYTICS=true
    volumes:
      - meilisearch_data:/meili_data
    healthcheck:
      test: ["CMD", "curl", "--fail", "http://localhost:7700/health"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  # catches every mail Oxynote sends and shows it at
  # http://localhost:8025.
  mailpit:
    image: axllent/mailpit:v1.31.0
    ports:
      - "8025:8025"
    restart: unless-stopped

volumes:
  oxynote_data:
  postgres_data:
  meilisearch_data:
```

</details>

Everything else, from search and email to object storage, the GitHub
and Slack apps, social login and Rubber Duck, is one variable away and
listed in [docker/prod/README.md](docker/prod/README.md). To run from
source, see [CONTRIBUTING.md](CONTRIBUTING.md).

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
- Freshness hooks: a block or a whole page declares what it depends on,
  and Oxynote tells you when that changes. Point a paragraph about a
  deploy procedure at the files on GitHub it describes, at the container
  image it names, at a web page, or at a date. When the target changes,
  the block is highlighted, the page loses freshness, and the maintainers
  get a reminder to re-read the text and approve it again.
- A page tree with sub pages, tags, full-text search, keyboard shortcuts
  for everything, light and dark themes.
- Workspaces with invitations, GitHub and Slack apps, and social login
  through GitHub, Google or Slack.

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

## Where this is going

Right now Oxynote covers the writing and reading side: pages, code, live
charts, *Split Documentation*, drafts with diffs and approvals, comments,
and Rubber Duck with an MCP server.

Next come the pieces that still live in other tools. Logs in the page,
so the chart, the log line and the explanation sit together. Feature
flags as blocks with an OpenFeature SDK, so a flag is documented,
reviewed and flipped in one place instead of a service nobody links to.
Admin actions as blocks, so the curl request nobody wants to paste into
Slack becomes a button with a review and an audit trail. Suggestions in
review, so a reviewer proposes the fix instead of describing it. Code
references, a side panel tying a block to the code behind it: the
function that emits the metric, the handler behind the endpoint, every
call site of a flag, indexed with tree-sitter so it needs no model at
runtime. Blocks for AI teams: prompts, evals and token usage next to the
RFC that changed them.

One surface for everything around a running system, simple for the
person who has to decide and for the agent helping them.

### Roadmap

**Pages**

- ✅ Real-time collaborative editor with headings, lists, code, callouts,
  images, files, Figma embeds and Mermaid diagrams
- ✅ Split Documentation block for Stripe-style API docs
- ✅ Page tree, tags, full-text search, keyboard shortcuts

**Live data in pages**

- ✅ Metric charts from Prometheus queries
- ✅ Metric charts from SQL queries on PostgreSQL, MySQL and MariaDB
- ✅ Metric grids, dashboard-style, with the explanation around them
- ⚪ Log blocks backed by Loki, next to the charts

**Review**

- ✅ Drafts with a diff of text and charts, approvals and merge into main
- ✅ Protected pages that only change through a merged draft
- ✅ Comments on text, diagrams and charts, with an inbox
- ⚪ Page history
- ⚪ Edit suggestions: a reviewer proposes the change, the author accepts
  it

**Operations**

- ✅ Freshness hooks: a block or page tracks the GitHub files, container
  image, web page or date it depends on and is flagged when that changes
- ⚪ Feature flag blocks with an OpenFeature SDK
- ⚪ Admin action blocks: a reviewed button instead of a curl request in
  Slack
- ⚪ Code references: a panel tying a block to the function, handler or
  call site behind it

**Humans and agents**

- ✅ Rubber Duck, the built-in assistant, with your choice of model
- ✅ MCP server, so Claude Code and other clients work on your pages
- ⚪ Blocks for AI teams: prompts, evals, token usage, agent topology

**Running it**

- ✅ One container plus PostgreSQL
- ✅ GitHub and Slack apps, social login

## FAQ

**Why not Notion plus Grafana?** I did exactly that. A Notion doc with a
link to the Grafana chart, and then (because you also want to get back)
a link from the chart to the doc. Then you keep both tabs open to check
they still say the same thing, and at some point you stop doing that,
and now you own a doc that lies a little. In Oxynote the metric chart is
a block inside the doc. Nothing to link, nothing to check.

**Why the name?** Internally we first called this "breathing docs".
Breathing needs oxygen, so oxy became the prefix, and note followed.

---

Oxynote is built by Simon ([@swithek](https://github.com/swithek)) and
David ([@davseby](https://github.com/davseby)), the authors of
[ttlcache](https://github.com/jellydator/ttlcache).
