"""Semantic and workload checks; run with `python test_core.py KIT_DIRECTORY`."""
import copy
import gzip
import json
import math
from pathlib import Path
import statistics
import sys
import time
import unittest

from arena import Arena
from bot import Bot, angle_delta, future_circle, intercept


KIT = Path(sys.argv[1] if len(sys.argv) > 1 else 'C:/Users/Window/Downloads/kit/x/mechfight-kit')
with gzip.open(KIT / 'check/stream.jsonl.gz', 'rt') as stream:
    MESSAGES = [json.loads(line) for line in stream]
HELLO = MESSAGES[0]
HELLO['map']['dir'] = str(KIT / 'arena')
OBSERVATIONS = [m for m in MESSAGES if m['type'] == 'obs']


class Semantics(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.bot = Bot(HELLO)

    def test_intercept_hits_moving_target(self):
        for v in ([12., -5., 0.], [-10., 12., 8.], [0., 0., 0.]):
            p = [120., 30., 4.5]
            o = [0., 0., 7.]
            q = intercept(o, p, v, 252.)
            t = math.dist(o, q) / 252.
            self.assertLess(math.dist(q, [p[i] + v[i] * t for i in range(3)]), 1e-6)

    def test_zone_prediction_before_and_during_shrink(self):
        z = dict(center=[0., 0.], radius=100., next_center=[10., 10.], next_radius=50., shrink_start=10., shrink_end=20.)
        self.assertEqual(future_circle(z, 0, 5), (0., 0., 100.))
        self.assertEqual(future_circle(z, 0, 15), (5., 5., 75.))
        z.update(center=[5., 5.], radius=75.)
        self.assertEqual(future_circle(z, 15, 20), (10., 10., 50.))

    def test_all_recorded_actions_finite_and_bounded(self):
        for obs in OBSERVATIONS:
            action = self.bot.act(obs)
            self.assertLessEqual(math.hypot(*action['move']), 1.00001)
            self.assertTrue(all(math.isfinite(v) for v in action['aim']))
            self.assertEqual(len(action['fire']), 2)
            self.assertTrue(all(isinstance(v, bool) for v in action['fire']))

    def test_prism_los_and_hole(self):
        a = object.__new__(Arena)
        a.valid, a.terrain, a.buckets = True, [], {}
        ring = [0., 0., 20., 0., 20., 20., 0., 20.]
        hole = [5., 5., 15., 5., 15., 15., 5., 15.]
        edges = [(r[i - 2], r[i - 1], r[i], r[i + 1]) for r in (ring, hole) for i in range(0, len(r), 2)]
        a.prisms = [(10., 20., [ring, hole], edges, (0., 20., 0., 20.))]
        a.buckets = {(0, 0): [0]}
        self.assertTrue(a.line_clear([-10, 10, 5], [30, 10, 5]), 'below bridge')
        self.assertFalse(a.line_clear([-10, 10, 15], [30, 10, 15]), 'through bridge solid')
        self.assertTrue(a.line_clear([7, 7, 15], [13, 13, 15]), 'courtyard hole')
        self.assertTrue(a.line_clear([-10, 10, 25], [30, 10, 25]), 'above roof')
        self.assertFalse(a.line_clear([2, 2, 5], [2, 2, 25]), 'vertical through slab')
        self.assertTrue(a.line_clear([32 - 1e-13, 10, 25], [32 + 1e-13, 10, 25]), 'tiny bucket crossing terminates')

    def test_gun_does_not_fire_before_aim_slews(self):
        obs = copy.deepcopy(OBSERVATIONS[0])
        obs['mechs'] = [dict(id=1, kind='mech', pos=[obs['self']['pos'][0] + 35, obs['self']['pos'][1], obs['self']['pos'][2]], vel=[0., 0., 0.], aim=[math.pi, 0.], hp=1400, cubes=0, weapons=['gun'])]
        self.bot.arena.line_clear = lambda a, b: True
        obs['self']['aim'] = [math.pi, 0.]
        act = {}
        self.bot.shooting(obs, act)
        self.assertFalse(act['fire'][0])
        obs['self']['aim'] = act['aim']
        act = {}
        self.bot.shooting(obs, act)
        self.assertTrue(act['fire'][0])
        del self.bot.arena.line_clear

    def test_laser_charges_while_gun_leads_then_releases_on_direct_aim(self):
        bot = Bot(HELLO)
        obs = copy.deepcopy(OBSERVATIONS[0])
        p = obs['self']['pos']
        enemy = dict(id=1, kind='mech', pos=[p[0] + 80, p[1], p[2]], vel=[0., 10., 0.], aim=[math.pi, 0.], hp=1400, cubes=0, weapons=['gun'])
        obs['mechs'] = [enemy]
        obs['self']['weapons'][1] = dict(kind='laser', cooldown=0., charge=.3, stats=HELLO['config']['weapons']['laser'])
        bot.arena.line_clear = lambda a, b: True
        bot.hostile_until[1] = 10.
        origin, target = [p[0], p[1], p[2] + 7], [p[0] + 80, p[1], p[2] + 4.5]
        lead = intercept(origin, target, enemy['vel'], 252.)
        obs['self']['aim'] = [math.atan2(lead[1] - p[1], lead[0] - p[0]), math.atan2(lead[2] - origin[2], math.hypot(lead[0] - p[0], lead[1] - p[1]))]
        act = {}
        bot.shooting(obs, act)
        self.assertTrue(all(act['fire']), 'gun shoots while laser charges')
        self.assertAlmostEqual(act['aim'][0], obs['self']['aim'][0])
        obs['self']['weapons'][1]['charge'] = .75
        obs['self']['aim'] = [0., math.atan2(-2.5, 80)]
        act = {}
        bot.shooting(obs, act)
        self.assertFalse(act['fire'][1], 'letting go releases laser')
        self.assertAlmostEqual(act['aim'][0], 0.)

    def test_loot_memory_expires_and_removes_missing_nearby_items(self):
        bot = Bot(HELLO)
        obs = copy.deepcopy(OBSERVATIONS[0])
        p = obs['self']['pos']
        item = dict(id=8001, kind='laser', pos=[p[0] + 100, p[1], p[2]])
        obs['pickups'] = [item]
        bot.remember(obs)
        obs['t'], obs['pickups'] = 1., []
        bot.remember(obs)
        self.assertIn(8001, bot.items['pickups'])
        obs['self']['pos'] = [p[0] + 95, p[1], p[2]]
        bot.arena.line_clear = lambda a, b: True
        bot.remember(obs)
        self.assertNotIn(8001, bot.items['pickups'])

    def test_cube_cluster_can_outweigh_a_single_nearby_cube(self):
        bot = Bot(HELLO)
        obs = copy.deepcopy(OBSERVATIONS[0])
        obs['self']['pos'] = [0., 0., 0.]
        obs['self']['weapons'][1] = dict(kind='laser', cooldown=0., charge=0., stats=HELLO['config']['weapons']['laser'])
        obs['cubes'] = [dict(id=5000, pos=[10., 0., 0.])] + [dict(id=5001 + i, pos=[30. + i * .2, 0., 0.]) for i in range(5)]
        bot.arena.ground = lambda *args: 0.
        bot.arena.clear_at = lambda *args: True
        bot.remember(obs)
        bot.select_goal(obs)
        self.assertEqual(bot.goal_kind, 'cube')
        self.assertGreaterEqual(bot.goal_id, 5001)

    def test_supply_capture_holds_position_and_suppresses_hop(self):
        bot = Bot(HELLO)
        bot.use_jets = True
        obs = copy.deepcopy(OBSERVATIONS[0])
        obs['self']['pos'] = [0., 0., 0.]
        obs['drops'] = [dict(id=20000, pos=[0., 0., 0.], radius=8., lands_in=0., capture_s=4., progress=3., holder=obs['self']['id'])]
        obs['mechs'] = [dict(id=1, kind='mech', pos=[100., 0., 0.], vel=[0., 0., 0.], aim=[math.pi, 0.], hp=1400., cubes=0, weapons=['gun'])]
        bot.hostile_until[1] = 10.
        bot.arena.ground = lambda *args: 0.
        bot.arena.clear_at = lambda *args: True
        bot.arena.walk_clear = lambda *args: True
        bot.arena.line_clear = lambda *args: True
        act = bot.act(obs)
        self.assertEqual(act['move'], [0., 0.])
        self.assertFalse(act['jump'])
        self.assertFalse(act['boost'])


def workload():
    bot = Bot(HELLO)
    obs = copy.deepcopy(OBSERVATIONS[0])
    obs['self']['pos'] = [0., 0., bot.arena.ground(0, 0)]
    obs['zone']['next_center'] = [200., 150.]
    obs['zone']['next_radius'] = 70.
    obs['zone']['shrink_start'] = 0.
    obs['zone']['shrink_end'] = 25.
    for k in range(15):
        yaw = k * math.tau / 15
        x, y = 65 * math.cos(yaw), 65 * math.sin(yaw)
        obs['mechs'].append(dict(id=k, kind='mech', pos=[x, y, bot.arena.ground(x, y)], vel=[-7 * math.sin(yaw), 7 * math.cos(yaw), 0.], aim=[yaw + math.pi, 0.], hp=1400, cubes=5, weapons=['gun', 'laser'], charging=k % 3 == 0))
    for k in range(24):
        yaw = k * math.tau / 24
        obs['projectiles'].append(dict(id=k, kind='gun', pos=[30 * math.cos(yaw), 30 * math.sin(yaw), 5.], vel=[-252 * math.cos(yaw), -252 * math.sin(yaw), 0.]))
    for k in range(80):
        yaw = k * math.tau / 80
        x, y = 42 * math.cos(yaw), 42 * math.sin(yaw)
        obs['cubes'].append(dict(id=5000 + k, pos=[x, y, bot.arena.ground(x, y)]))
    for k in range(24):
        yaw = k * math.tau / 24
        x, y = 55 * math.cos(yaw), 55 * math.sin(yaw)
        obs['pickups'].append(dict(id=8000 + k, kind='laser' if k % 2 else 'lightning', pos=[x, y, bot.arena.ground(x, y)]))
    times = []
    for k in range(200):
        obs['t'], obs['tick'] = k * .1, k * 2
        start = time.perf_counter()
        bot.act(obs)
        times.append((time.perf_counter() - start) * 1000)
    times.sort()
    print('Crowded decision time ms:', dict(p50=round(statistics.median(times), 2), p99=round(times[int(len(times)*.99)], 2), maximum=round(max(times), 2)))


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(Semantics))
    workload()
    sys.exit(not result.wasSuccessful())
