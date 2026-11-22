# DECISIONS - Mech Royale (Builders) - 16-way 3D battle royale bot

Curated current truth (short). History lives in `log.txt`. Add: decision, why, date/time, who.



- 2026-11-22 11:51 codex: bot core uses bounded 4m ground A* plus local threat/zone/cover scoring; opportunistic safe resources, lead-aim, survival first. Avoid blind next-center movement. 96 practice matches confirmed; live status at 12:34 now lists 6 submissions/hour (setup track page said 4).

- 2026-11-22 12:08 codex: exp/combat isolated worktree for simultaneous gun/laser and measured engagement A/B; main stays reviewable. Ground-only enclosure/roof limits tracked for later flight.

- 2026-11-22 12:20 codex: test economy exploration and selective retaliation; avoid tagging passive squad members. exp/combat533e479 is baseline before this experiment (48 local seeds score2.71).

- 2026-11-22 12:24 codex: exp/kite isolated range/boost/healing experiment; gun causes dominate (29/48), so prioritize spacing before rare enclosed-roof flight.


- 2026-11-22 12:40 codex: exp/dps isolates premature reload removal; jet candidate dcad276 is frozen/reviewed, waiting for entry3 scoring to finish.
