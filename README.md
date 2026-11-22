# Mech Royale — Last Light Go v2

Native Go bot for the `mechfight/1` protocol, using only the Go standard library.
MIT licensed. The root Dockerfile builds a static Linux/amd64 executable and
runs it in a scratch image as uid 65534, with no Python or runtime dependencies.

The initial tactical reference was our local **Buildathon-lab-v8 Apex engine**. The Go
refactor implements its quiet opening, squad hunt and selective FFA phases,
target persistence, low-health finishing, cover seeking, lightning avoidance,
cube/weapon economy, upgrade priorities, projectile lead and laser release.
The reference's Go source directories were absent, so these are newly written
Go modules, rather than a copied Go binary or a Python subprocess wrapper.

## Layout

- `cmd/mechbot`: JSON-lines lifecycle, newest-observation queue draining.
- `internal/proto`: typed messages and config/stat access.
- `internal/world`: terrain interpolation, polygon/overhang columns, body-aware
  movement, connected navigation, bounded A* and cover search.
- `internal/bot`: stateful phase controller, movement/economy and fire control.
- Existing Python sources remain local reference work; the Go image includes
  only the native executable.

Rules and weapon numbers are read from `hello.config` and live `self.stats`.
Map geometry is loaded from `hello.map.dir` (normally `/arena`). Trees do not
block shots. Enemy body sizes scale with cubes; live self dimensions are used
without double scaling. Early foreign deaths are counted as deaths, correcting
the reference's frame counter and excluding late zone attrition. Simultaneous gun fire is checked against the final
shared torso aim when laser release takes control.

The refactor is a new candidate, not an assertion of score equivalence to the
Python reference or an official practice-score improvement.

The v2 update independently implements concepts reviewed in the user-supplied
**Buildathon-main11/Buildathon-main** reference: loadout-dependent combat range,
laser firing clocks from observed beams, witnessed enemy-versus-enemy FFA
detection, rear-arc exposure costs, earlier armed pursuit, reachable supply-drop
captures, clustered cube values and cover reloads. Gun prediction reduces its
horizon at strafe reversals and uses the current weapon cooldown; laser aiming
takes over the shared torso close to release so the gun keeps its lead longer.
The protocol runner also preserves the match process after a decision panic.
Reference bot source and its archived reports were not copied into this repo.

**This v2 revision was pushed without builds, tests, vet, simulation or the
conformance checker, as explicitly requested by the user due to the deadline.**
The previous candidate reportedly scored about 5.3; no new score is claimed.

## Build and validate

```sh
go test ./...
go build -trimpath -o mechbot ./cmd/mechbot
python3 /path/to/mechfight-kit/check/check.py -- ./mechbot
docker build -t last-light-go .
python3 /path/to/mechfight-kit/check/check.py --map-dir /arena -- \
  docker run --rm -i --network none --cpus 1 --memory 512m --read-only \
  --tmpfs /tmp:rw,size=64m --user 65534:65534 \
  -v /path/to/mechfight-kit/arena:/arena:ro last-light-go
```

The conformance checker measures transport correctness and timing; it does not
predict battle placement. Submit the exact tested public commit for scoring.

Historical validation of commit `a5016d0` only: all 23 behavior/geometry tests passed; the official checker
answers 300/300 observations within budget under the one CPU, 512 MB, read-only,
network-disabled Docker restrictions (p99 7.3 ms, maximum 9.6 ms). A separate
200-decision stress run with 15 enemies, 100 projectiles and an enlarged mech on
the real map had no late decisions (maximum 10.1 ms). Timing varies by machine.

## Sources and license

- The user-supplied Buildathon-lab-v8 Python engine was the initial tactical reference.
- The user-supplied Buildathon-main11 Go archive was read for mechanism ideas;
  the v2 changes are original implementations within our existing modules.
- The official Builders starter kit provides the protocol, schema, checker and
  mounted map. The retained `mechfight.py` is its Python wrapper.
- Previous local experiments used the MIT-licensed headless simulator from
  `anthonyteti/buildathon-mech-royale`; simulator/reference repositories are not
  included in the Go runtime image. No other team's bot code was used in this
  refactor.
- The native Go implementation uses no third-party packages.
