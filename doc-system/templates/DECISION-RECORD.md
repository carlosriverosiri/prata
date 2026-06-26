---
id: DR-NNNN
title: <short decision title>
status: ACCEPTED            # PROPOSED | ACCEPTED | SUPERSEDED-BY:DR-NNNN | REJECTED
date: <YYYY-MM-DD>
valid-as-of-commit: <git-short-sha>   # machine-checkable freshness anchor
deciders: [<human>, <ai-model>]
supersedes: []
constants: []               # ids into CONSTANTS.md that this decision sets
invariants: []              # ids of must-not-change rules this establishes
tags: []
---

# DR-NNNN — <title>

## Context

<The forces. What problem, what constraints, what was observed. Be concrete:
quote the real symptom/repro that forced a choice.>

## Options considered

> For EACH option — including the chosen one — fill all four fields.
> **Rejected options are MANDATORY.** An empty rejected list fails the lint and,
> more importantly, robs a future rebuilder of the dead-end knowledge.

### Option A — <name>
- What it is: <one line>
- Pro: <…>
- Con / why rejected: <the concrete reason it died — not "we preferred B">
- Evidence: <repro, measurement, link, or the throwaway experiment that killed it>

### Option B — <name>
- Why rejected: <…>

### Option C (CHOSEN) — <name>
- Why chosen: <…>

## Decision

<The chosen option, stated as an imperative a rebuilder implements directly.>

## Reasoning dialogue

<The actual back-and-forth that produced the decision — preserved verbatim or
lightly edited. THIS is the asset a future model cannot reconstruct from the
outcome alone: the wrong hypotheses, the intermediate positions, the
"we tried X, it failed because Y" narrative.>

## Invariants established (must-not-change)

- INV-<name>: <the rule + the failure mode if violated>. <load-bearing?>

## Constants set by this decision

| id | value | unit | why THIS value |
| --- | --- | --- | --- |
| `<name>` | <value> | <unit> | <reasoning> |

## Consequences

- Positive: <…>
- Negative / accepted cost: <…>
- Follow-ups / opened questions: <…>

<!-- A lighter DR-lite (front-matter + Context + Options + Decision) is allowed for
small decisions, but `status`, ≥1 REJECTED option, and `valid-as-of-commit` are
always required. -->
