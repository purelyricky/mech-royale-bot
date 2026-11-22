"""Inspect individual local practice results without writing private match logs.

Used to estimate whether early non-player deaths can safely identify an FFA.
The local MIT simulator is a test dependency in ignored ref/, never shipped.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import statistics
import subprocess


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('variant')
    ap.add_argument('--n', type=int, default=24)
    ap.add_argument('--seed', type=int, default=701)
    ap.add_argument('--jobs', type=int, default=3)
    ap.add_argument('--foreign100', type=int, default=4)
    ap.add_argument('--foreign180', type=int, default=6)
    ap.add_argument('--kit', type=Path, default=Path('C:/Users/Window/Downloads/kit/x/mechfight-kit'))
    args = ap.parse_args()
    root = Path(__file__).resolve().parent
    env = dict(os.environ, CFG=str(root / 'ref/config_default.json'), KIT=str(args.kit.resolve()), OPP_REGISTRY=str(root / 'ref/opponents.json'))

    def run(seed):
        spec = dict(seed=seed, mode='practice', ours=[dict(ext=args.variant, label=args.variant)], maxT=480)
        result = subprocess.run(['node', 'sim/run.ts', '--child', json.dumps(spec)], cwd=root / 'ref/anthonyteti_buildathon-mech-royale', env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode:
            return dict(seed=seed, failed=True)
        try:
            game = json.loads(result.stdout.strip().splitlines()[-1])
            own = next(r for r in game['results'] if r['label'] == args.variant)
        except (ValueError, KeyError, StopIteration, IndexError):
            return dict(seed=seed, failed=True)
        foreign = []
        own_causes = 0
        for r in game['results']:
            cause = str(r.get('cause', 'alive'))
            if r['id'] == own['id'] or cause.startswith('alive'):
                continue
            if cause.rsplit(':', 1)[-1] == args.variant:
                own_causes += 1
            elif r['deathT'] <= own['deathT']:
                foreign.append(r['deathT'])
        before100 = sum(t < 100 for t in foreign)
        before180 = sum(t < 180 for t in foreign)
        return dict(seed=seed, score=16 - own['place'], kills=own['kills'], cubes=own.get('cubes', 0), death_t=round(own['deathT'], 1), foreign100=before100, foreign180=before180, detector_false_positive=before100 >= args.foreign100 or before180 >= args.foreign180, own_cause_count=own_causes)

    with ThreadPoolExecutor(max_workers=args.jobs) as pool:
        rows = list(pool.map(run, range(args.seed, args.seed + args.n)))
    good = [r for r in rows if not r.get('failed')]
    print('sample summary', json.dumps(dict(variant=args.variant, thresholds=[args.foreign100, args.foreign180], n=len(good), failed=len(rows) - len(good), score=round(statistics.mean(r['score'] for r in good), 3) if good else None, kills=round(statistics.mean(r['kills'] for r in good), 3) if good else None, cubes=round(statistics.mean(r['cubes'] for r in good), 3) if good else None, false_ffa_detections=sum(r['detector_false_positive'] for r in good))))
    print('foreign early deaths', json.dumps([(r['seed'], r['foreign100'], r['foreign180']) for r in good]))
    print('lowest placements', json.dumps(sorted(good, key=lambda r: r['score'])[:5]))
    raise SystemExit(bool(len(good) != len(rows)))


if __name__ == '__main__':
    main()
