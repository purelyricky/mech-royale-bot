"""Invoke the locally installed MIT simulator with safe native argument passing.

The simulator and opponent registry are local tools in ignored ref/; never shipped.
These estimates are useful for paired comparisons, not official score predictions.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('variant', help='name in the local opponent registry')
    ap.add_argument('--n', type=int, default=24)
    ap.add_argument('--seed', type=int, default=101)
    ap.add_argument('--jobs', type=int, default=3)
    ap.add_argument('--against', help='optional paired control from the local registry')
    ap.add_argument('--mode', choices=['practice', 'ffa'], default='practice')
    ap.add_argument('--kit', type=Path, default=Path('C:/Users/Window/Downloads/kit/x/mechfight-kit'))
    args = ap.parse_args()
    root = Path(__file__).resolve().parent
    env = dict(os.environ)
    env.update(CFG=str(root / 'ref/config_default.json'), KIT=str(args.kit.resolve()), OPP_REGISTRY=str(root / 'ref/opponents.json'))
    spec = dict(ext=args.variant)
    if not args.against:
        spec['label'] = args.variant
    cmd = ['node', 'sim/run.ts', args.mode, '--n', str(args.n), '--seed', str(args.seed), '--jobs', str(args.jobs), '--a', json.dumps(spec, separators=(',', ':'))]
    if args.against:
        cmd += ['--b', json.dumps(dict(ext=args.against), separators=(',', ':'))]
    result = subprocess.run(cmd, cwd=root / 'ref/anthonyteti_buildathon-mech-royale', env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    print('\n'.join(result.stdout.splitlines()[-5:]))
    raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
