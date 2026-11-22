"""Aggregate temporary external-bot counters from local simulation results."""
import argparse
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import statistics
import subprocess


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--variant', default='instrument')
    ap.add_argument('--n', type=int, default=24)
    ap.add_argument('--seed', type=int, default=2001)
    ap.add_argument('--jobs', type=int, default=2)
    args = ap.parse_args()
    root = Path(__file__).resolve().parent
    env = dict(os.environ, CFG=str(root/'ref/config_default.json'), KIT='C:/Users/Window/Downloads/kit/x/mechfight-kit', OPP_REGISTRY=str(root/'ref/opponents.json'))
    def run(seed):
        spec = dict(seed=seed, mode='practice', ours=[dict(ext=args.variant, label=args.variant)], maxT=480)
        p = subprocess.run(['node','sim/run.ts','--child',json.dumps(spec)], cwd=root/'ref/anthonyteti_buildathon-mech-royale', env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if p.returncode:
            raise RuntimeError(f'Local simulator failed for seed {seed}')
        r = next(r for r in json.loads(p.stdout.strip().splitlines()[-1])['results'] if r['label'] == args.variant)
        if not r.get('botProfile', {}).get('total'):
            raise RuntimeError(f'Missing counters for seed {seed}')
        return r
    with ThreadPoolExecutor(max_workers=args.jobs) as pool:
        games = list(pool.map(run, range(args.seed, args.seed+args.n)))
    c = Counter()
    for g in games:
        c.update(g['botProfile'])
    if sum(v for k,v in c.items() if k.startswith('state_')) != c['total']:
        raise RuntimeError('Shooting states are not an exhaustive partition')
    print('Instrumented games', args.n, 'observations', c['total'], 'life_s', round(sum(g['deathT'] for g in games),1), 'score', round(statistics.mean(16-g['place'] for g in games),3), 'kills', round(statistics.mean(g['kills'] for g in games),3), 'gun_shots_per_life_s', round(sum(g['shots'] for g in games)/sum(g['deathT'] for g in games),3))
    print('Exclusive shooting states % life', json.dumps({k:round(100*v/c['total'],2) for k,v in c.most_common() if k.startswith('state_')}))
    print('Overlapping context % life', json.dumps({k:round(100*v/c['total'],2) for k,v in c.items() if k != 'total' and not k.startswith('state_')}))
    print('Per game [score,kills,death_s,observations,shots]', json.dumps([[16-g['place'],g['kills'],g['deathT'],g['botProfile']['total'],g['shots']] for g in games]))


if __name__ == '__main__':
    main()
