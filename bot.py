"""Survival-oriented Mech Royale pilot. No runtime dependencies outside Python."""
from __future__ import annotations

import math
import random
import time

from arena import Arena
import mechfight as mf


def distance(a, b):
    return math.hypot(a[0] - b[0], a[1] - b[1])


def angle_delta(a, b):
    return (a - b + math.pi) % (2 * math.pi) - math.pi


def unit(dx, dy):
    length = math.hypot(dx, dy)
    return (dx / length, dy / length) if length > .01 else (0., 0.)


def intercept(origin, position, velocity, speed):
    """Constant-velocity projectile intercept; the bullet has world-frame speed."""
    r = [position[i] - origin[i] for i in range(3)]
    a = sum(v * v for v in velocity) - speed * speed
    b = 2 * sum(r[i] * velocity[i] for i in range(3))
    c = sum(v * v for v in r)
    disc = b * b - 4 * a * c
    t = math.sqrt(c) / speed
    if disc >= 0 and abs(a) > 1e-9:
        roots = [v for v in ((-b + math.sqrt(disc)) / (2 * a), (-b - math.sqrt(disc)) / (2 * a)) if v >= 0]
        if roots:
            t = min(roots)
    elif abs(b) > 1e-9 and abs(a) <= 1e-9:
        t = max(0., -c / b)
    t = min(2., t)
    return [position[i] + velocity[i] * t for i in range(3)]


def future_circle(zone, now, future):
    start = max(now, zone['shrink_start'])
    f = max(0., min(1., (future - start) / max(.001, zone['shrink_end'] - start)))
    return (zone['center'][0] + f * (zone['next_center'][0] - zone['center'][0]),
            zone['center'][1] + f * (zone['next_center'][1] - zone['center'][1]),
            zone['radius'] + f * (zone['next_radius'] - zone['radius']))


class Bot:
    name = 'Oval Sentinel'
    use_jets = False

    def __init__(self, hello):
        self.config = hello['config']
        self.rng = random.Random(hello['seed'])
        self.arena = Arena(hello['map']['dir'], self.config['mech'])
        self.goal = None
        self.goal_kind = 'zone'
        self.goal_id = None
        self.goal_until = -1.
        self.route = []
        self.route_goal = None
        self.route_until = -1.
        self.nav_shape = None
        self.previous_move = (0., 0.)
        self.last_damage = -100.
        self.last_combat = -100.
        self.last_hp = None
        self.last_positions = []
        self.threats = []
        self.memory = {}
        self.items = {'pickups': {}, 'cubes': {}, 'crates': {}}
        self.focus_id = None
        self.hostile_until = {}
        self.aim_streak = {}
        self.last_obs_t = -.1
        self.visited = {}
        self.jet_started = -100.
        self.jet_altitude = 0.
        self.jet_cooldown = 0.
        self.ffa = False
        self.initial_players = self.config['match'].get('mechs', 16)
        self.own_kills = set()
        self.orbit = self.rng.choice((-1, 1))
        self.scan_phase = self.rng.uniform(-math.pi, math.pi)
        self.deadline = math.inf

    def remember(self, obs):
        now = obs['t']
        dt = min(.2, max(0., now - self.last_obs_t))
        self.last_obs_t = now
        own_pos = obs['self']['pos']
        self.visited[(round(own_pos[0] / 25), round(own_pos[1] / 25))] = now
        for actor in obs['mechs']:
            self.memory[actor['id']] = (now, actor)
            if actor['kind'] == 'mech':
                toward = math.atan2(own_pos[1] - actor['pos'][1], own_pos[0] - actor['pos'][0])
                aimed = abs(angle_delta(actor['aim'][0], toward)) < .3
                streak = self.aim_streak.get(actor['id'], 0.)
                self.aim_streak[actor['id']] = min(1., streak + dt) if aimed else 0.
                if self.aim_streak[actor['id']] >= .25 or (aimed and actor.get('charging')):
                    self.hostile_until[actor['id']] = now + 3
        self.threats = []
        for idx, (seen, actor) in list(self.memory.items()):
            age = now - seen
            if age > 9 or actor.get('hp', 1) <= 0:
                del self.memory[idx]
                continue
            copy = dict(actor)
            copy['weight'] = (1. if actor['kind'] == 'mech' else .23) * max(.2, 1 - age / 10)
            if actor['kind'] == 'mech' and self.hostile_until.get(actor['id'], -1) < now and distance(actor['pos'], own_pos) > 55:
                copy['weight'] *= .65
            # The server tracks real positions for five seconds; only extrapolate our own old memory.
            copy['pos'] = [actor['pos'][i] + actor['vel'][i] * min(age, .7) for i in range(3)]
            self.threats.append(copy)
        self.threats.sort(key=lambda e: distance(e['pos'], obs['self']['pos']))
        for kind, remembered in self.items.items():
            visible_ids = {item['id'] for item in obs[kind]}
            for item in obs[kind]:
                remembered[item['id']] = (now, dict(item))
            for idx, (seen, item) in list(remembered.items()):
                d = distance(item['pos'], obs['self']['pos'])
                p = item['pos']
                me = obs['self']
                eye = [me['pos'][0], me['pos'][1], me['pos'][2] + me['stats'].get('eye_height', 8)]
                expired = now - seen > (45 if kind == 'pickups' else 20)
                missing_nearby = idx not in visible_ids and d < 40 and self.arena.line_clear(eye, [p[0], p[1], p[2] + 1])
                if expired or missing_nearby:
                    del remembered[idx]
        for event in obs['events']:
            if event['type'] == 'damaged':
                self.last_damage = now
                self.last_combat = now
                if event.get('by') is not None:
                    self.hostile_until[event['by']] = now + 12
            elif event['type'] == 'hit':
                self.last_combat = now
            elif event['type'] == 'kill':
                self.memory.pop(event['target'], None)
                self.own_kills.add(event['target'])
        foreign_deaths = max(0, self.initial_players - obs['alive'] - len(self.own_kills))
        if (now < 100 and foreign_deaths >= 4) or (now < 180 and foreign_deaths >= 6):
            self.ffa = True
        if self.ffa:
            for enemy in self.threats:
                if enemy['kind'] == 'mech' and self.hostile_until.get(enemy['id'], -1) < now and distance(enemy['pos'], own_pos) > 55:
                    enemy['weight'] /= .65

    def risk(self, point, now, cover=False, z=None):
        total = 0.
        for enemy in self.threats[:10]:
            d = distance(point, enemy['pos'])
            weight = enemy['weight']
            if enemy['kind'] == 'npc' and d > 65:
                weight *= .2
            power = 1 + .05 * min(enemy.get('cubes', 0), 10)
            if enemy['kind'] == 'mech':
                power *= .55 + .45 * min(1., enemy.get('hp', 1400) / max(1., enemy.get('max_hp', 1400)))
            r = weight * power * (17000 / (d + 28) ** 2 + max(0., 65 - d) * .025)
            if enemy.get('charging'):
                ex, ey = point[0] - enemy['pos'][0], point[1] - enemy['pos'][1]
                off = abs(angle_delta(math.atan2(ey, ex), enemy['aim'][0]))
                if off < .12:
                    r += 4 * weight
            if cover and d < 250 and enemy in self.threats[:5]:
                ez = enemy['pos'][2] + 7 * (1 + .05 * enemy.get('cubes', 0))
                pz = self.arena.ground(*point[:2]) if z is None else z
                if not self.arena.line_clear([point[0], point[1], pz + 5], [enemy['pos'][0], enemy['pos'][1], ez]):
                    r *= .16
            total += r
        return total

    def zone_cost(self, point, obs, horizon, margin=8.):
        cx, cy, radius = future_circle(obs['zone'], obs['t'], obs['t'] + horizon)
        outside = max(0., distance(point, (cx, cy)) - max(0., radius - margin))
        return outside * .18 + outside * outside * .012

    def select_goal(self, obs):
        me, now, zone = obs['self'], obs['t'], obs['zone']
        pos = me['pos']
        speed = me['stats'].get('walk_speed', 11.7)
        candidates = [(pos[:2], 'zone', None, 0.)]
        if self.goal is not None:
            reward = -2.4 if self.goal_kind == 'explore' and distance(pos, self.goal) > 8 else -.35
            table = {'cube': 'cubes', 'weapon': 'pickups', 'crate': 'crates'}.get(self.goal_kind)
            valid = table is None or self.goal_id in self.items[table]
            if self.goal_kind == 'drop':
                valid = any(d['id'] == self.goal_id for d in obs.get('drops', []))
            if valid:
                candidates.append((self.goal, self.goal_kind, self.goal_id, reward))
        # Local alternatives preserve freedom rather than driving every spawn to the center.
        for r in (35., 80.):
            for k in range(12):
                yaw = self.scan_phase + k * math.tau / 12
                point = pos[0] + r * math.cos(yaw), pos[1] + r * math.sin(yaw)
                explore = now < 65 and r == 35 and me['hp'] > me['max_hp'] * .85 and not any(self.hostile_until.get(e['id'], -1) > now for e in self.threats if e['kind'] == 'mech')
                fresh = now - self.visited.get((round(point[0] / 25), round(point[1] / 25)), -100) > 35
                candidates.append((point, 'explore' if explore else 'zone', None, -1.8 if explore and fresh else 0.))
        cx, cy, cr = future_circle(zone, now, now + 18)
        candidates.append(((cx, cy), 'zone', None, 0.))
        nr = zone['next_radius']
        if nr > 15:
            yaw0 = math.atan2(pos[1] - zone['next_center'][1], pos[0] - zone['next_center'][0])
            for k in range(-3, 4):
                yaw = yaw0 + k * .38
                candidates.append(((zone['next_center'][0] + .65 * nr * math.cos(yaw), zone['next_center'][1] + .65 * nr * math.sin(yaw)), 'zone', None, 0.))
        nearest = distance(pos, self.threats[0]['pos']) if self.threats else 1e6
        peaceful = nearest > 90 and now - self.last_damage > 2
        hunter_near = any(e['kind'] == 'mech' and (distance(pos, e['pos']) < (140 if self.ffa else 60) or (self.hostile_until.get(e['id'], -1) > now and distance(pos, e['pos']) < 110)) for e in self.threats)
        roof = me['grounded'] and pos[2] > self.arena.ground(*pos[:2]) + 3
        def reachable_item(item):
            return abs(item['pos'][2] - self.arena.ground(*item['pos'][:2])) < 4 or (roof and abs(item['pos'][2] - pos[2]) < 4)
        healthy = me['hp'] > me['max_hp'] * .72
        lightning = next((w for w in me['weapons'] if w and w['kind'] == 'lightning'), None)
        close_mechs = [e for e in self.threats if e['kind'] == 'mech' and distance(pos, e['pos']) < 140]
        if healthy and len(close_mechs) <= 1:
            for enemy in obs['mechs']:
                d = distance(pos, enemy['pos'])
                if d > 220 or enemy.get('hp', 1) <= 0:
                    continue
                npc = enemy['kind'] == 'npc'
                shorting = bool(lightning and me['energy'] > 30 and not npc and 'laser' in enemy.get('weapons', []) and 'lightning' not in enemy.get('weapons', []) and self.hostile_until.get(enemy['id'], -1) > now and d < 130)
                if not npc and enemy.get('hp', 1400) > 450 and not shorting:
                    continue
                if self.ffa and obs['alive'] > 6 and not npc and enemy.get('hp', 1400) > 250:
                    continue
                if npc and (not peaceful or d > 100):
                    continue
                others = sum(1 for other in close_mechs if other['id'] != enemy['id'] and distance(other['pos'], enemy['pos']) < 110)
                if others > 1:
                    continue
                # Approach isolated enemies while healthy, then strafe at a useful gun range.
                standoff = 42. if npc else (85. if 'lightning' in enemy.get('weapons', []) else 75.)
                if shorting:
                    standoff = .82 * self.weapon_stats(lightning)['range']
                ux, uy = unit(pos[0] - enemy['pos'][0], pos[1] - enemy['pos'][1])
                point = enemy['pos'][0] + ux * standoff, enemy['pos'][1] + uy * standoff
                reward = -4. if npc else -5. - max(0., 800 - enemy.get('hp', 1400)) * .004
                if shorting:
                    reward = -10.
                candidates.append((point, 'engage', enemy['id'], reward))
        # Resources are deliberately local; crossing the map to chase a cube is rarely safe.
        cube_items = [item for _, item in self.items['cubes'].values()]
        for cube in cube_items:
            d = distance(pos, cube['pos'])
            if d < 110 and reachable_item(cube):
                count = sum(1 for other in cube_items if distance(cube['pos'], other['pos']) < 8 and abs(cube['pos'][2] - other['pos'][2]) < 4)
                useful = min(count, max(0, 10 - me['cubes']))
                value = min(22., 5. + useful * 3.) if useful else 1.
                candidates.append((cube['pos'][:2], 'cube', cube['id'], -value / (1 + d / 90)))
        held = {w['kind'] for w in me['weapons'] if w}
        for _, pickup in self.items['pickups'].values():
            d = distance(pos, pickup['pos'])
            if d < 190 and pickup['kind'] not in held and reachable_item(pickup):
                value = {'laser': 12., 'lightning': 4., 'mortar': 0., 'gun': 0.}.get(pickup['kind'], 0)
                if len(held) >= 2:
                    value = max(0., value - 4)
                if value:
                    candidates.append((pickup['pos'][:2], 'weapon', pickup['id'], -value / (1 + d / 100)))
        if not hunter_near and me['hp'] > me['max_hp'] * .75 and me['cubes'] < 10:
            for _, crate in self.items['crates'].values():
                d = distance(pos, crate['pos'])
                if d < 95 and reachable_item(crate):
                    candidates.append((crate['pos'][:2], 'crate', crate['id'], -6. / (1 + d / 80)))
            for npc in obs['mechs']:
                d = distance(pos, npc['pos'])
                if npc['kind'] == 'npc' and d < 110 and reachable_item(npc):
                    ux, uy = unit(pos[0] - npc['pos'][0], pos[1] - npc['pos'][1])
                    point = npc['pos'][0] + ux * 55, npc['pos'][1] + uy * 55
                    candidates.append((point, 'farm', npc['id'], -8. / (1 + d / 110)))
        for drop in obs.get('drops', []):
            d = distance(pos, drop['pos'])
            occupied = any(e['kind'] == 'mech' and distance(e['pos'], drop['pos']) < drop.get('radius', 8) + 3 for e in self.threats)
            if d < 280 and drop.get('lands_in', 0) < max(8., d / speed * 1.35) and (drop.get('holder') in (None, me['id'])) and not occupied:
                value = 12. + (1 - me['hp'] / me['max_hp']) * 16
                candidates.append((drop['pos'][:2], 'drop', drop['id'], -value / (1 + d / 180)))
        ranked = []
        for point, kind, idx, reward in candidates:
            if not self.arena.clear_at(*point) and not (roof and distance(pos, point) < 90 and self.arena.clear_at(point[0], point[1], pos[2])):
                continue
            d = distance(pos, point)
            travel = d / max(1., speed)
            # Arrival and a few seconds of safety; future next-circle pressure rises with time.
            score = self.zone_cost(point, obs, travel + 8, min(12., cr * .12))
            deadline = max(1., zone['shrink_end'] - now)
            excess = max(0., distance(point, zone['next_center']) - max(0., nr - 10))
            score += excess * .025 * min(3., (travel + 12) / deadline)
            # Start a long evacuation before the shrinking boundary reaches us. Route length
            # can substantially exceed straight-line distance on this building-dense map.
            required_time = (d * 1.25 + excess * 1.6) / max(1., speed) + 12
            score += max(0., required_time - deadline) ** 2 * .22
            score += self.risk(point, now) * (1.7 if me['hp'] < me['max_hp'] * .6 else 1.)
            score += d * .012 + reward
            ranked.append((score, point, kind, idx))
        ranked.sort(key=lambda c: c[0])
        best = None
        # Cover is more expensive than a distance field: refine only the strongest candidates.
        for score, point, kind, idx in ranked[:7]:
            if best is not None and time.perf_counter() > self.deadline - .012:
                break
            if self.threats:
                score += (self.risk(point, now, cover=True) - self.risk(point, now)) * 1.3
            if best is None or score < best[0]:
                best = score, point, kind, idx
        if best:
            self.goal = tuple(best[1][:2])
            self.goal_kind, self.goal_id = best[2:]
        else:
            self.goal, self.goal_kind, self.goal_id = tuple(zone['next_center']), 'zone', None
        self.goal_until = now + 1.3

    def waypoint(self, obs):
        pos, now = obs['self']['pos'], obs['t']
        radius = obs['self']['stats'].get('radius', self.config['mech']['radius']) + .25
        height = obs['self']['stats'].get('height', self.config['mech']['height'])
        shape = round(radius, 2), round(height, 2)
        if shape != self.nav_shape:
            self.route, self.route_until, self.nav_shape = [], -1., shape
        if distance(pos, self.goal) < 5:
            return self.goal
        if self.arena.walk_clear(pos, self.goal, radius, height):
            self.route = []
            return self.goal
        if now >= self.route_until or self.route_goal is None or distance(self.route_goal, self.goal) > 12 or not self.route:
            self.route = self.arena.path(pos, self.goal, deadline=self.deadline - .012, radius=radius, height=height)
            self.route_goal = self.goal
            self.route_until = now + 1.8
        # Skip only short, verified segments so grid corners cannot cut through buildings.
        while len(self.route) > 1 and distance(pos, self.route[0]) < 5:
            self.route.pop(0)
        for k in range(min(5, len(self.route) - 1), 0, -1):
            if self.arena.walk_clear(pos, self.route[k], radius, height):
                self.route = self.route[k:]
                break
        return self.route[0] if self.route else self.goal

    def movement(self, obs, waypoint):
        me, now = obs['self'], obs['t']
        x, y, z = me['pos']
        stats = me['stats']
        speed = stats.get('walk_speed', 11.7)
        radius = stats.get('radius', 2.5) + .25
        height = stats.get('height', 9.)
        wanted = unit(waypoint[0] - x, waypoint[1] - y)
        current_dist = distance((x, y), waypoint)
        arrival_speed = min(1., current_dist / max(1., speed * .85))
        directions = [(wanted[0] * arrival_speed, wanted[1] * arrival_speed), self.previous_move, (0., 0.)]
        directions += [(math.cos(k * math.tau / 24), math.sin(k * math.tau / 24)) for offset in range(3) for k in range(offset, 24, 3)]
        close = [e for e in self.threats if distance(e['pos'], (x, y)) < 130]
        ranked = []
        for ux, uy in directions:
            if len(ranked) >= 7 and time.perf_counter() > self.deadline - .005:
                break
            horizon = .85
            px = x + ux * speed * horizon + me['vel'][0] * .08
            py = y + uy * speed * horizon + me['vel'][1] * .08
            if not self.arena.walk_clear(me['pos'], (px, py), radius, height):
                continue
            score = (distance((px, py), waypoint) - current_dist) * .14
            cx, cy, safe_radius = future_circle(obs['zone'], now, now + 1.8)
            # A route around a wall can initially lead away from the circle. A quadratic
            # local zone cost would pin us against the wall while the zone kills us.
            score += max(0., distance((px, py), (cx, cy)) - max(0., safe_radius - 5)) * .035
            score += self.risk((px, py), now) * (2 if me['hp'] < .6 * me['max_hp'] else 1.2)
            if close:
                # Moving tangentially to a gun line frustrates leading and telegraphed lasers.
                enemy = close[0]
                ex, ey = unit(enemy['pos'][0] - x, enemy['pos'][1] - y)
                lateral = abs(ux * ey - uy * ex)
                score -= .45 * lateral
                if distance((x, y), enemy['pos']) < 55:
                    score += max(0., ux * ex + uy * ey) * 1.4
                if ux == 0 and uy == 0:
                    score += 1.2
            score += .1 * ((ux - self.previous_move[0]) ** 2 + (uy - self.previous_move[1]) ** 2)
            # Dodge visible projectiles at their closest approach over the next half second.
            for p in obs['projectiles'][:24]:
                rx, ry = p['pos'][0] - x, p['pos'][1] - y
                vx, vy = p['vel'][0] - ux * speed, p['vel'][1] - uy * speed
                vv = vx * vx + vy * vy
                t = max(0., min(.55, -(rx * vx + ry * vy) / max(1., vv)))
                dx, dy = rx + vx * t, ry + vy * t
                pz = p['pos'][2] + p['vel'][2] * t
                if p['kind'] == 'mortar':
                    pz -= .5 * self.config['weapons']['mortar'].get('gravity', 22) * t * t
                if z - 1 <= pz <= z + height + 2 and t > .015:
                    rr = radius + (7 if p['kind'] == 'mortar' else 1.2)
                    score += max(0., rr * rr - dx * dx - dy * dy) * .18
            ranked.append((score, ux, uy, px, py))
        ranked.sort()
        best = None
        for score, ux, uy, px, py in ranked[:5]:
            if best is not None and time.perf_counter() > self.deadline - .005:
                break
            if close:
                score += (self.risk((px, py), now, cover=True, z=z) - self.risk((px, py), now)) * 1.5
            if best is None or score < best[0]:
                best = score, ux, uy
        move = best[1:] if best else wanted
        if current_dist < 3 and self.goal_kind in ('zone', 'drop') and not close:
            move = (0., 0.)
        energy = me['energy']
        nearest = distance(me['pos'], close[0]['pos']) if close else 1e6
        boost = me['grounded'] and energy > 30 and (not me['in_zone'] or (nearest < 85 and me['hp'] < .8 * me['max_hp']))
        if me['in_zone'] and energy < me['max_energy'] * .38:
            boost = False
        # Stuck recovery is deliberate, not an unconditional jump every few seconds.
        self.last_positions.append((now, x, y))
        self.last_positions = [p for p in self.last_positions if now - p[0] < 1.4]
        stuck = len(self.last_positions) > 8 and distance(self.last_positions[0][1:], (x, y)) < 1 and current_dist > 7
        jump = bool(stuck and me['grounded'] and energy > 30)
        if not me['grounded']:
            boost = bool(stuck and energy > 25 and me['vel'][2] < 4)
        self.previous_move = move
        return list(move), bool(boost), jump

    def weapon_stats(self, weapon):
        return {**self.config['weapons'][weapon['kind']], **weapon.get('stats', {})}

    def visible_aim_point(self, actor, origin):
        kind = actor['kind']
        if kind == 'npc':
            height = self.config['npc']['height']
            fractions = (.55, .75, .92, .3)
        elif kind == 'crate':
            height = self.config['crates']['height']
            fractions = (.5, .75, .9)
        else:
            scale = 1 + self.config['cubes'].get('size_per_cube', .05) * actor.get('cubes', 0)
            height = self.config['mech']['height'] * scale
            fractions = (.5, .7, .9, .3)
        for i, fraction in enumerate(fractions):
            if i and time.perf_counter() > self.deadline - .001:
                break
            point = [actor['pos'][0], actor['pos'][1], actor['pos'][2] + height * fraction]
            if self.arena.line_clear(origin, point):
                return point, i > 0
        return None, False

    def shooting(self, obs, action):
        me = obs['self']
        pos, now, stats = me['pos'], obs['t'], me['stats']
        origin = [pos[0], pos[1], pos[2] + stats.get('muzzle_height', 7)]
        weapons = me['weapons']
        laser_slot = next((i for i, w in enumerate(weapons) if w and w['kind'] == 'laser'), None)
        laser = weapons[laser_slot] if laser_slot is not None else None
        laser_ready = laser and laser.get('cooldown', 0) <= .05
        charging = bool(laser and laser.get('charge', 0) > 0)
        target = None
        candidates = []
        for actor in obs['mechs']:
            if candidates and time.perf_counter() > self.deadline - .001:
                break
            d = distance(pos, actor['pos'])
            if actor['kind'] == 'npc':
                danger = any(e['kind'] == 'mech' and (distance(pos, e['pos']) < (150 if self.ffa else 60) or (self.hostile_until.get(e['id'], -1) > now and distance(pos, e['pos']) < 110)) for e in self.threats)
                if d > 95 or me['hp'] < .75 * me['max_hp'] or danger:
                    continue
                score = d * .5 + 30 if self.goal_kind == 'farm' and actor['id'] == self.goal_id else d + 65
            else:
                hostile = self.hostile_until.get(actor['id'], -1) > now
                if not self.ffa and not hostile and d > 46 and actor.get('hp', 1400) > 400:
                    continue
                score = d * .5 + actor.get('hp', 1400) * .075 - (18 if actor['id'] == self.focus_id else 0)
                aim_yaw = actor.get('aim', [0, 0])[0]
                to_me = math.atan2(pos[1] - actor['pos'][1], pos[0] - actor['pos'][0])
                if abs(angle_delta(aim_yaw, to_me)) > math.pi / 2:
                    score -= 14  # rear hit: x1.5 damage
                if laser_slot is not None and 'mortar' in (actor.get('weapons') or []):
                    score -= 12  # laser cook-off bonus
                score += 10 * sum(1 for o in obs['mechs'] if o['kind'] == 'mech' and o['id'] != actor['id'] and distance(o['pos'], actor['pos']) < 35)
            if d < 280:
                point, partial = self.visible_aim_point(actor, origin)
                if point is not None:
                    candidates.append((score + (5 if partial else 0), actor, point))
        if candidates:
            target = min(candidates, key=lambda c: c[0])[1:]
        # Don't aggro distant drones or keep resetting regen for a crate while hurt.
        if target is None and me['hp'] >= .8 * me['max_hp'] and now - self.last_damage > 1.5:
            crates = sorted(obs['crates'], key=lambda c: distance(pos, c['pos']))
            for crate in crates[:3]:
                if distance(pos, crate['pos']) < 95 and (self.goal_kind == 'crate' or distance(pos, crate['pos']) < 22):
                    proxy = dict(crate, kind='crate', vel=[0., 0., 0.])
                    point, _ = self.visible_aim_point(proxy, origin)
                    if point is not None:
                        target = (proxy, point)
                        break
        fire = [False, False]
        if target:
            actor, point = target
            self.focus_id = actor['id']
            d = distance(pos, point)
            # Lead the gun while the laser charges, then switch to hitscan aim for release.
            gun = next((w for w in weapons if w and w['kind'] == 'gun'), None)
            gun_point = intercept(origin, point, actor.get('vel', [0, 0, 0]), self.weapon_stats(gun)['speed']) if gun else point
            desired_charge = self.weapon_stats(laser)['charge'] if laser else 0.
            if laser:
                ls = self.weapon_stats(laser)
                power = 1 + self.config['cubes'].get('damage_per_cube', .04) * min(me['cubes'], self.config['cubes'].get('stat_cap', 10))
                multiplier = 1 + ls.get('cookoff', .6) if 'mortar' in actor.get('weapons', []) else 1.
                damage = ls['damage'] * power * multiplier
                fraction = (actor.get('hp', 1400) * 1.12 / max(1., damage) - ls.get('min_frac', .35)) / (1 - ls.get('min_frac', .35))
                desired_charge = min(ls['charge'], max(ls.get('min_charge', .15), fraction * ls['charge']))
            release_phase = bool(laser and laser_ready and laser.get('charge', 0) >= desired_charge - .1)
            aim_point = point if release_phase else gun_point
            yaw = math.atan2(aim_point[1] - pos[1], aim_point[0] - pos[0])
            pitch = math.atan2(aim_point[2] - origin[2], distance(pos, aim_point))
            action['aim'] = [yaw, max(stats.get('min_pitch', -.7), min(stats.get('max_pitch', 1.3), pitch))]
            yaw_error = abs(angle_delta(me['aim'][0], yaw))
            pitch_error = abs(me['aim'][1] - pitch)
            target_radius = 1.5 if actor['kind'] == 'crate' else 2.2 * (1 + .05 * actor.get('cubes', 0))
            tolerance = max(.012, math.atan2(target_radius, max(1., d)))
            aligned = yaw_error < tolerance + .025 and pitch_error < max(.03, tolerance)
            # At low HP, distant chip damage prevents our own regeneration and gives away cover.
            abstain = me['hp'] < .55 * me['max_hp'] and d > 110 and actor.get('hp', 1400) > 350
            if self.ffa and me['hp'] < .7 * me['max_hp'] and d > 100 and actor.get('hp', 1400) > 350:
                abstain = True
            for i, w in enumerate(weapons):
                if not w:
                    continue
                ws, kind = self.weapon_stats(w), w['kind']
                if kind == 'gun':
                    gun_yaw = math.atan2(gun_point[1] - pos[1], gun_point[0] - pos[0])
                    gun_pitch = math.atan2(gun_point[2] - origin[2], distance(pos, gun_point))
                    gun_aligned = abs(angle_delta(me['aim'][0], gun_yaw)) < tolerance + .025 and abs(me['aim'][1] - gun_pitch) < max(.03, tolerance)
                    fire[i] = bool(gun_aligned and d < ws['range'] - 3 and not abstain)
                    if not fire[i] and w.get('ammo', 12) < ws['mag'] * .65 and not w.get('reload', 0):
                        action['reload'] = True
                elif kind == 'laser':
                    full = desired_charge
                    charge = w.get('charge', 0)
                    # The release fires immediately: hold while lining up, release at full charge.
                    fire[i] = bool(d < ws['range'] and not abstain and (charge < full - .035 or not aligned) and w.get('cooldown', 0) <= .05)
                    if charge == 0 and (yaw_error > .35 or pitch_error > .3):
                        fire[i] = False
                elif kind == 'lightning':
                    fire[i] = bool(d < ws['range'] - 1 and yaw_error < math.radians(ws['cone_deg']) * .7 and me['energy'] > 18)
                elif kind == 'mortar' and d > ws['splash_radius'] + stats.get('radius', 2.5) + 8 and not charging and not laser_ready:
                    # Use a low ballistic arc; do not mix a gun's pitch with shell launch pitch.
                    vx = ws['speed']
                    g = ws.get('gravity', stats.get('gravity', 22))
                    moving = [point[j] + actor.get('vel', [0, 0, 0])[j] * min(2.5, d / vx) for j in range(3)]
                    md = distance(pos, moving)
                    dz = moving[2] - (pos[2] + stats.get('height', 9) + .5)
                    disc = vx ** 4 - g * (g * md * md + 2 * dz * vx * vx)
                    # Prioritize the gun unless the shell can actually fire this decision.
                    if disc >= 0 and w.get('cooldown', 0) <= .1 and not abstain:
                        mortar_pitch = math.atan((vx * vx - math.sqrt(disc)) / max(.001, g * md))
                        mortar_yaw = math.atan2(moving[1] - pos[1], moving[0] - pos[0])
                        if stats.get('min_pitch', -.7) <= mortar_pitch <= stats.get('max_pitch', 1.3):
                            action['aim'] = [mortar_yaw, mortar_pitch]
                            fire = [False, False]
                            fire[i] = bool(abs(angle_delta(me['aim'][0], mortar_yaw)) < .05 and abs(me['aim'][1] - mortar_pitch) < .045)
            mf.draw.line(origin, point, '#f66')
        else:
            if self.threats:
                e = self.threats[0]
                action['aim'] = [math.atan2(e['pos'][1] - pos[1], e['pos'][0] - pos[0]), 0.]
            elif obs['sounds']:
                relevant = [s for s in obs['sounds'] if s['kind'] in ('gun', 'laser', 'laser_charge', 'lightning', 'mortar', 'step', 'boost')]
                if relevant:
                    action['aim'] = [min(relevant, key=lambda s: ({'near': 0, 'mid': 1, 'far': 2}[s['dist']], s['age']))['bearing'], 0.]
            if 'aim' not in action:
                if self.goal_kind in ('weapon', 'crate', 'cube', 'farm'):
                    action['aim'] = [math.atan2(self.goal[1] - pos[1], self.goal[0] - pos[0]), 0.]
                else:
                    # A slow periodic full sweep detects loot outside the forward wedge.
                    action['aim'] = [self.scan_phase + now * .7, 0.]
            for w in weapons:
                if w and w['kind'] == 'gun' and w.get('ammo', 12) < self.weapon_stats(w)['mag'] and not w.get('reload', 0):
                    action['reload'] = True
        action['fire'] = fire

    def act(self, obs):
        self.deadline = time.perf_counter() + .027
        me, now = obs['self'], obs['t']
        self.remember(obs)
        if self.goal is None or now >= self.goal_until:
            self.select_goal(obs)
        waypoint = self.waypoint(obs)
        move, boost, jump = self.movement(obs, waypoint)
        action = {'tick': obs['tick'], 'move': move, 'boost': boost, 'jump': jump}
        capturing = False
        for drop in obs.get('drops', []):
            occupied = any(e['kind'] == 'mech' and distance(e['pos'], drop['pos']) < drop.get('radius', 8) + 1 for e in self.threats)
            if drop.get('lands_in', 0) <= .1 and distance(me['pos'], drop['pos']) < drop.get('radius', 8) - 2 and abs(me['pos'][2] - drop['pos'][2]) < 20 and not occupied:
                action.update(move=[0., 0.], boost=False, jump=False)
                capturing = True
                break
        hostile = any(e['kind'] == 'mech' and self.hostile_until.get(e['id'], -1) > now and distance(e['pos'], me['pos']) < 160 for e in self.threats)
        stats = me['stats']
        if self.use_jets and not capturing and me['grounded'] and hostile and now >= self.jet_cooldown and me['energy'] > me['max_energy'] * .65:
            eye = [me['pos'][0], me['pos'][1], me['pos'][2] + stats.get('height', 9) + .5]
            overhead = [eye[0], eye[1], eye[2] + 18]
            if self.arena.line_clear(eye, overhead):
                self.jet_started, self.jet_cooldown = now, now + 5.5
                self.jet_altitude = me['pos'][2] + 14.
                action.update(jump=True, boost=True)
        elif self.use_jets and not capturing and not me['grounded'] and now - self.jet_started < 5.5:
            action['boost'] = bool(now - self.jet_started < 1.7 and me['pos'][2] < self.jet_altitude and me['energy'] > 25)
        self.shooting(obs, action)
        offer = obs.get('offer')
        if offer:
            priorities = {'nanites': 100, 'armor': 96, 'plating': 94, 'servos': 92, 'reactor': 84, 'jets': 80, 'laser_damage': 78, 'laser_charge': 76, 'gun_damage': 72, 'gun_rate': 70, 'gun_mag': 68, 'capacitor': 66, 'gun_velocity': 64, 'lightning_range': 62, 'lightning_efficiency': 60, 'sensors': 52, 'gyros': 48}
            if me['cubes'] < 6:
                priorities.update(laser_damage=118, gun_damage=114, laser_charge=112, gun_rate=110, gun_mag=108, servos=104)
            action['pick'] = max(range(len(offer['choices'])), key=lambda i: priorities.get(offer['choices'][i]['id'], 20))
        held = {w['kind'] for w in me['weapons'] if w}
        reach = self.config['loot']['pickup_radius']
        near = [p for p in obs['pickups'] if p['kind'] not in held and p['kind'] != 'mortar' and distance(me['pos'], p['pos']) < reach and abs(me['pos'][2] - p['pos'][2]) <= 4]
        if near:
            values = {'gun': 0, 'mortar': 1, 'lightning': 2, 'laser': 3}
            pickup = max(near, key=lambda p: values.get(p['kind'], 0))
            empty = next((i for i, w in enumerate(me['weapons']) if w is None), None)
            if empty is not None:
                action['take'] = pickup['id']
            else:
                old = min(range(2), key=lambda i: values.get(me['weapons'][i]['kind'], 0) if me['weapons'][i]['kind'] != 'gun' else 100)
                weapon = me['weapons'][old]
                if values.get(pickup['kind'], 0) > values.get(weapon['kind'], 0) and not weapon.get('charge', 0):
                    action.update(take=pickup['id'], drop=old)
        mf.draw.point([waypoint[0], waypoint[1], self.arena.ground(*waypoint)], '#4cf', 4)
        mf.draw.label(f'{"FFA " if self.ffa else ""}{self.goal_kind} hp {me["hp"]:.0f}')
        return action


if __name__ == '__main__':
    mf.run(Bot)
