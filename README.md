# bounty

Finds open GitHub issues that carry a reward, ranks them against your stack, and
returns the ones worth your time.

Go with the standard library only: `net/http`, `encoding/json`, `context`. No
dependencies, one binary — which is what makes it behave the same on your
machine and on a CI runner.

---

## What it does, and what it does not

It **hunts and ranks**. You decide and you solve.

It does not write code, does not open pull requests and does not submit
anything. An agent that reads an issue off the internet and submits a patch is
exactly the data → execution chain this architecture exists to prevent. The
model proposes; the program does not obey.

---

## Where the bounties actually are

The obvious strategy — search GitHub for `label:bounty` — does not work, and it
is worth writing down why, because the answer cost a day of investigation.

**Measured on 2026-09-07**, a label-based collection returned 28 issues. Twenty-
three came from a single synthetic repository, with duplicated issues ("reissue
via #743") and a `reward:` label carrying no value. The other five came from
one-person personal repositories. Zero real bounties.

The cause is economic: a reward attracts people farming rewards. Since coding
agents became cheap, standing up a repository with a hundred fake issues and a
bounty label became a way to show up in search.

**The platforms have also left.** Algora and Polar, the two services that funded
open-source issues, are out of that market:

- **Algora** pivoted to hiring. Its bounty route is still live at
  `/api/trpc/bounty.list`, answers `200`, and returns `{"items":[]}`
  unconditionally — the query logic is commented out in
  [its own source](https://github.com/algora-io/algora).
- **Polar** became a payments platform for products. Its API has no issue,
  funding or reward endpoint at all.

What is left is the trail on GitHub itself. `/bounty` is the command that
creates a reward on an issue, and it stays in the comment history. Comparing
three searches on the same day:

| query | results | what comes back |
|---|---|---|
| `label:"💎 Bounty"` | 558 | farms, forks, agent playgrounds |
| `label:"bounty"` | 4,281 | farms and crypto micro-bounties |
| **`"/bounty" in:comments`** | **17,997** | `asterisk/asterisk`, `vllm-project/vllm-omni`, `Tarsnap/tarsnap` |

Labels became noise because anyone can create one. A comment carrying the
command is the trace of someone who actually used the platform.

**One honest limit:** GitHub tokenises the query and drops the slash, so this
matches the word "bounty" in comments rather than the command specifically. It
is a much better filter than labels, not an exact one — and it does not recover
the amount, which lives in the comment body and is not fetched yet.

---

## The quality filter

Finding the right search is half of it. The other half is refusing the farms.

[`internal/rank/quality.go`](internal/rank/quality.go) cuts on four signals:

| cut | default | what it buys |
|---|---|---|
| stars | 200 | a project someone besides the owner uses, whose maintainer has a reputation to lose by not paying |
| days since last push | 90 | a living project. A bounty on a still repository is almost never paid, because nobody is there to merge it |
| forks | rejected | a fork carrying a bounty is the commonest farm disguise: it looks like the famous project, but whoever pays is not there |
| repository name | pattern list | a real project is rarely called `something-bounties`; it has a name of its own |

On a real run this drops **76 of 100** results, and what survives is
`asterisk/asterisk`, `Tarsnap/tarsnap`, `vllm-project/vllm-omni`,
`tenstorrent/tt-metal`.

There is a cost, and it should be said: the more famous the project, the more
people compete for the same bounty. If the hunt starts returning only issues
with ten comments on them, the star cut is the first to come down.

The per-reason account of what was dropped is returned as a warning. A filter
that discards silently is a filter nobody notices is miscalibrated.

---

## Usage

```sh
echo '{"contract":1,"project":"bounties","path":"","since":null,
       "config":{"terms":["/bounty"],"languages":["go","php","python"]}}' | bin/bounty
```

### Configuration

| key | type | default | what it does |
|---|---|---|---|
| `terms` | list | `["/bounty"]` | terms searched in comment history; one query each |
| `languages` | list | — | your stack; a match scores +40 |
| `min_stars` | int | 200 | quality cut |
| `max_idle_days` | int | 90 | quality cut |
| `limit` | int | 25 | how many to return |
| `per_page` | int | 50 | results per query, 100 max |
| `timeout_seconds` | int | 60 | total budget |

### Credentials

The token comes from `IODE_GITHUB_TOKEN`. The engine forwards anything prefixed
`IODE_` to the subprocess, and never reads a credential file itself. Public read
access is enough.

Without a token the **Search API** allows 10 requests a minute; with one, 30.
That is far tighter than the core API's 5,000 an hour — which is why repository
metadata is fetched there, and only the issue search comes out of the Search
API.

### Ranking

Deterministic. String matching and counting, no model heuristics, so the same
input always produces the same output.

| signal | weight |
|---|---|
| matches one of your languages | +40 |
| amount ≥ $1000 / ≥ 200 / > 0 | +40 / +25 / +10 |
| opened this week / this month | +20 / +10 |
| open for over a year | −20 |
| more than 15 comments | −15 |

Ties break toward the most recent.

### Exit codes

| code | when |
|---|---|
| 0 | success, warnings included |
| 1 | **every** query failed; the engine will retry |
| 2 | unknown contract version |
| 3 | empty stdin, invalid JSON, `since` without an offset, or bad config |

---

## Development

```sh
make check     # build + vet + test
make install   # copy to ~/.config/iode/adapters/bounty
```

No test touches the network: GitHub responses come from an `httptest` server on
loopback, so the real HTTP client is exercised rather than a stand-in.
