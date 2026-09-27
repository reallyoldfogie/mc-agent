# Combat Threat Model

This document defines the first decision-layer contract for PvP/combat. It is independent of Minecraft packets so it can be tested without a live server.

## Classification

`combat.Classify` maps tracked registry names into `Player`, `Hostile`, `Neutral`, or `Unknown`. Players are the highest default category. The hostile list covers standard aggressive mobs; server-specific overrides can be applied later.

## Eligibility

The default ranker excludes removed, invisible, and unknown entities. Neutral entities are excluded unless explicitly enabled. This prevents accidental attacks on passive entities.

## Ranking

Targets are ordered by category, distance, remaining health ratio, and entity ID as a deterministic final tie-breaker. The numeric score is an implementation detail; controllers should consume the ordered result and re-rank when their snapshot becomes stale.
