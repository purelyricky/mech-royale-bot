"""Ground-physics zone probe, not a combat simulator or a score predictor.

Usage: python nav_probe.py [KIT_DIRECTORY] [SEEDS]
The official conformance checker cannot detect a bot stuck against a building.
This independent closed-loop probe applies acceleration, walls and zone damage.
"""
import copy
import gzip
import json
import math
from pathlib import Path
import random
import sys
import time
from unittest.mock import patch

from arena import Arena
from bot import Bot, distance, future_circle


def run(kit, seeds, first_seed=0):
    with gzip.open(kit / 'check/stream.jsonl.gz', 'rt') as f:
        hello, base = json.loads(next(f)), json.loads(next(f))
    hello['map']['dir'] = str(kit / 'arena')
    arena = Arena(hello['map']['dir'], hello['config']['mech'])
    stats = hello['config']['mech']
    rng = random.Random(81823)
    open_cells = [i for i, free in enumerate(arena.grid) if free]
    results = []
    for seed in range(first_seed + seeds):
        hello['seed'] = seed
        with patch('bot.Arena', return_value=arena):
            bot = Bot(hello)
        timings = {}
        for method_name in ('select_goal', 'waypoint', 'movement', 'shooting'):
            method = getattr(bot, method_name)
            def timed(*args, _method=method, _name=method_name):
                began = time.perf_counter()
                result = _method(*args)
                timings[_name] = (time.perf_counter() - began) * 1000
                return result
            setattr(bot, method_name, timed)
        obs = copy.deepcopy(base)
        start = arena.grid_point(rng.choice(open_cells))
        goal = arena.grid_point(rng.choice(open_cells))
        while distance(start, goal) < 250:
            goal = arena.grid_point(rng.choice(open_cells))
        if seed < first_seed:
            continue
        x, y = start
        vx, vy = 0., 0.
        obs['self']['pos'] = [x, y, arena.ground(x, y)]
        obs['self']['hp'] = 1400.
        # A deliberately faster 90s closure spanning a large portion of the map.
        obs['zone'] = dict(center=list(start), radius=distance(start, goal) + 60, next_center=list(goal), next_radius=12., shrink_start=25., shrink_end=90., dps=100.)
        initial_zone = copy.deepcopy(obs['zone'])
        outside_s, blocked_s, late = 0., 0., 0
        max_decision = 0.
        for tick in range(1000):
            t = tick * .1
            cx, cy, radius = future_circle(initial_zone, 0, t)
            obs['t'], obs['tick'] = t, tick * 2
            obs['zone']['center'], obs['zone']['radius'] = [cx, cy], radius
            me = obs['self']
            me['in_zone'] = distance((x, y), (cx, cy)) <= radius
            me['pos'], me['vel'] = [x, y, arena.ground(x, y)], [vx, vy, 0.]
            now = time.perf_counter()
            timings.clear()
            action = bot.act(obs)
            if seeds == 1 and tick % 50 == 0:
                print('trace', json.dumps(dict(t=round(t, 1), pos=[round(x, 1), round(y, 1)], goal=bot.goal, waypoint=bot.route[:2], goal_kind=bot.goal_kind, move=action['move'], zone_radius=round(radius, 1))), flush=True)
            elapsed = time.perf_counter() - now
            if seeds == 1 and elapsed > .04:
                print('slow', json.dumps(dict(t=t, pos=[x, y], goal=bot.goal, ms=timings)), flush=True)
            max_decision = max(max_decision, elapsed)
            late += elapsed > .05
            speed = stats['boost_speed'] if action.get('boost') and me['energy'] > 0 else stats['walk_speed']
            before = x, y
            for step in range(2):
                tx, ty = [v * speed for v in action['move']]
                dvx, dvy = tx - vx, ty - vy
                change = math.hypot(dvx, dvy)
                accel = min(1., stats['ground_accel'] * .05 / max(.0001, change))
                vx, vy = vx + dvx * accel, vy + dvy * accel
                px, py = x + vx * .05, y + vy * .05
                # Independent axis slide, using the actual mech radius rather than navigation margin.
                z = arena.ground(x, y)
                if arena.clear_at(px, y, arena.ground(px, y), stats['radius'], stats['height']) and arena.ground(px, y) - z < stats['step_up']:
                    x = px
                else:
                    vx = 0.
                z = arena.ground(x, y)
                if arena.clear_at(x, py, arena.ground(x, py), stats['radius'], stats['height']) and arena.ground(x, py) - z < stats['step_up']:
                    y = py
                else:
                    vy = 0.
            if distance(before, (x, y)) < .02 and math.hypot(*action['move']) > .1:
                blocked_s += .1
            if not me['in_zone']:
                outside_s += .1
            me['energy'] = max(0., me['energy'] - stats['boost_drain'] * .1) if action.get('boost') else min(100., me['energy'] + stats['energy_regen'] * .1)
            me['aim'] = action['aim']
        result = dict(seed=seed, start=[round(v, 1) for v in start], goal=[round(v, 1) for v in goal], end_distance=round(distance((x, y), goal), 1), outside_s=round(outside_s, 1), blocked_s=round(blocked_s, 1), max_ms=round(max_decision * 1000, 1), late=late)
        results.append(result)
        print(json.dumps(result), flush=True)
    print('summary', json.dumps(dict(seeds=len(results), failed=sum(r['end_distance'] > 15 for r in results), total_outside_s=round(sum(r['outside_s'] for r in results), 1), total_blocked_s=round(sum(r['blocked_s'] for r in results), 1))))


if __name__ == '__main__':
    run(Path(sys.argv[1] if len(sys.argv) > 1 else 'C:/Users/Window/Downloads/kit/x/mechfight-kit'), int(sys.argv[2]) if len(sys.argv) > 2 else 8, int(sys.argv[3]) if len(sys.argv) > 3 else 0)
