# DECISIONS - Mech Royale

Current truth, updated 2026-11-22 13:50 by codex. Detailed evidence and decisions live in the append-only shared `log.txt`.

- Best accepted practice entry is economy commit `30fffdd`, score **4.61**, still selected. Combined economy/kill/FFA `a57efb3` scored 4.35. Never select a worse entry without a new instruction from the user.
- Frozen next candidate is `11a076f`: conservative FFA mode, lead's target ranking, partial-body LOS aim, and current-body-size navigation. Restricted Linux checker answered 300/300; all 15 semantic checks passed; grown-body dense workload max 25.75 ms. Lead peer review is required before submitting.
- Core is dependency-free Python: bounded 4 m A*, terrain/prism LOS, predictive zone movement, local threat/projectile steering, safe cube/NPC/crate economy, and simultaneous gun/laser handling. Keep the 27 ms optional-work deadline inside the official 50 ms response budget.
- Economy was the strongest accepted improvement. Global combat hops, range-aware healing/kiting, reload-only removal, and offense-threshold 3/11 sweeps failed paired comparisons; keep jets disabled and offense upgrades through six cubes.
- FFA detection excludes own kills and requires at least four foreign deaths before 100 s or six before 180 s. No late-zone inference. It permits selective final-mode shooting while avoiding early healthy opponents; twelve mixed local games improved placement, but self-play is not an official prediction.
- Body dimensions in `self.stats` are already cube-scaled; do not scale them twice. Navigation lazily caches at most four body shapes instead of rebuilding a full occupancy grid every pickup.
- Prioritize measured damage and kills for subsequent experiments. Codex owns isolated `exp/duel` range/strafe testing against frozen `11a076f`; no experiment changes main during packaging.
- Live status reports six submissions/hour (setup track said four). Submit sequentially, keep a reserve, wait for pending scoring, and cross-review each candidate. Identical code retains its first builders-4 score.
- Lead handles GitHub packaging, browser scoring, and submissions. Codex's browser and Windows fallback share an unavailable native pipe; retry periodically and continue CLI work through the shared log.
- Runtime image is pinned Python 3.13 Bookworm, standard library only. Official kit wrapper and MIT local simulator are attributed; simulator/reference code is ignored and never shipped.
