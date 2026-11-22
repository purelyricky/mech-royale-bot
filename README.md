# Mech Royale bot (Builders Buildathon 2026, team FigTeam707)

A pure-Python (standard library only) bot for the `mechfight/1` protocol. MIT licensed.

## Layout
- `bot.py` - decision loop: zone-aware movement scored by predicted risk, gun lead aiming, laser charge/release, lightning, mortar ballistics, loot and upgrade choices.
- `arena.py` - map model built from `/arena/v2/world.json` and `terrain.i16`: height sampling, building polygons, line of sight, A* on an inflated grid.
- `mechfight.py` - protocol wrapper copied unchanged from the official Builders kit.
- `Dockerfile` - runs `bot.py` on `python:3.13-slim-bookworm`.

## Third-party code and ideas
- `mechfight.py` is the organizers' starter-kit wrapper (unchanged).
- No other team's code is included. Ideas were read from public write-ups and re-implemented from scratch: reCurse's Halite II post-mortem (survival mode, coarse-then-refine), Robocode minimum-risk movement, and public READMEs/docs of other Buildathon teams (FFA vs practice detection, laser charge/release with the gun, avoiding lightning carriers).
- For local A/B testing only (not shipped in the image) we used the MIT-licensed headless simulator from `anthonyteti/buildathon-mech-royale`.

## Test
```sh
python3 check/check.py -- python3 -u bot.py
docker build -t mech-bot .
```
