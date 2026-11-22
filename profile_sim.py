"""Summarize local simulator combat phases without retaining match logs."""
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
    ap.add_argument('--n', type=int, default=12)
    ap.add_argument('--seed', type=int, default=1901)
    ap.add_argument('--jobs', type=int, default=2)
    args = ap.parse_args()
    root = Path(__file__).resolve().parent
    env = dict(os.environ, CFG=str(root / 'ref/config_default.json'), KIT='C:/Users/Window/Downloads/kit/x/mechfight-kit', OPP_REGISTRY=str(root / 'ref/opponents.json'), PRACLOG='1', SHOTLOG='1')

    def run(seed):
        spec = dict(seed=seed, mode='practice', ours=[dict(ext=args.variant, label=args.variant)], maxT=480)
        p = subprocess.run(['node', 'sim/run.ts', '--child', json.dumps(spec)], cwd=root / 'ref/anthonyteti_buildathon-mech-royale', env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if p.returncode:
            raise RuntimeError(f'Local simulator failed for seed {seed}')
        result = next(r for r in json.loads(p.stdout.strip().splitlines()[-1])['results'] if r['label'] == args.variant)
        rows = result.get('plog', [])
        first = lambda col, value: next((r[0] for r in rows if r[col] > value), None)
        secondary = next((r[0] for r in rows if len(r[9]) > 1), None)
        under_fire = [r for r in rows if r[5] > 0]
        result['phase_summary'] = dict(seed=seed, score=16-result['place'], kills=result['kills'], death=result['deathT'], dealt=result['dealt'], first_dealt=first(11, 0), first_taken=first(10, 0), first_kill=first(8, 0), first_secondary=secondary, stopped_under_fire_s=round(.1 * sum(r[1] < 1 for r in under_fire), 1), under_fire_s=round(.1 * len(under_fire), 1), multi_attack_s=round(.1 * sum(r[5] >= 2 for r in rows), 1), roof_s=round(.1 * sum(r[2] > 4 for r in rows), 1))
        return result

    with ThreadPoolExecutor(max_workers=args.jobs) as pool:
        games = list(pool.map(run, range(args.seed, args.seed + args.n)))
    phases = [g['phase_summary'] for g in games]
    keys = [k for k in phases[0] if k != 'seed']
    print('Phase means', json.dumps({k: round(statistics.mean(r[k] for r in phases if r[k] is not None), 2) if any(r[k] is not None for r in phases) else None for k in keys}))
    print('Individual phases', json.dumps(phases))
    shots = [s for g in games for s in g.get('shotLog', [])]
    print('Shot summary', json.dumps(dict(n=len(shots), hit_fraction=round(statistics.mean(s[0] for s in shots), 3) if shots else None, median_initial_range=statistics.median(s[3] for s in shots) if shots else None, mean_target_speed=round(statistics.mean(s[4] for s in shots), 2) if shots else None)))
    for label, col, edges in [('initial_range', 3, [0,40,70,100,140,10000]), ('target_speed', 4, [0,2,7,12,1000]), ('velocity_change_in_flight', 2, [0,2,5,10,1000])]:
        bins = {}
        for low,high in zip(edges,edges[1:]):
            group = [s for s in shots if low <= s[col] < high]
            bins[f'{low}-{high}'] = dict(n=len(group), hit_pct=round(100*statistics.mean(s[0] for s in group),1) if group else None)
        print('Shot bins', label, json.dumps(bins))
    for g in sorted(games, key=lambda g: (g['phase_summary']['score'], g['deathT']))[:2]:
        print('Poor-match phases [t,speed,roof,seen_by,nearest,recent_attackers,nearby,hp,kills,loadout,taken,dealt]', g['phase_summary']['seed'], json.dumps([r for i,r in enumerate(g['plog']) if i % 100 == 0]))


if __name__ == '__main__':
    main()
