"""Static arena geometry and bounded ground navigation (standard library only)."""
from __future__ import annotations

from array import array
import heapq
import json
import math
from pathlib import Path
import sys
import time


def inside(x, y, ring):
    hit = False
    ax, ay = ring[-2:]
    for i in range(0, len(ring), 2):
        bx, by = ring[i:i + 2]
        if (ay > y) != (by > y) and x < (bx - ax) * (y - ay) / (by - ay) + ax:
            hit = not hit
        ax, ay = bx, by
    return hit


def segment_dist2(x, y, ax, ay, bx, by):
    dx, dy = bx - ax, by - ay
    length = dx * dx + dy * dy
    u = max(0.0, min(1.0, ((x - ax) * dx + (y - ay) * dy) / length)) if length else 0.0
    return (x - ax - u * dx) ** 2 + (y - ay - u * dy) ** 2


class Arena:
    cell = 4.0
    bucket = 32.0

    def __init__(self, directory, mech):
        self.valid = False
        self.prisms = []
        self.buckets = {}
        self.ring_rows = {}
        self.edge_grids = []
        self.terrain = array('h')
        self.bounds = (-512., 555., -333., 388.)
        self.grid = bytearray()
        self.radius = mech.get('radius', 2.5) + 0.6
        self.height = mech.get('height', 9.)
        self.step_up = mech.get('step_up', 1.2)
        self.max_slope = math.tan(math.radians(mech.get('max_slope_deg', 50)))
        try:
            root = Path(directory) / 'v2'
            with (root / 'world.json').open(encoding='utf-8') as f:
                world = json.load(f)
            self.t = world['terrain']
            self.terrain.frombytes((root / self.t['file']).read_bytes())
            if sys.byteorder != 'little':
                self.terrain.byteswap()
            b = world['bounds']
            self.bounds = b['minX'], b['maxX'], b['minY'], b['maxY']
            for raw in world['prisms']:
                ring = raw['ring']
                rings = [ring] + raw.get('holes', [])
                edges = []
                for r in rings:
                    ax, ay = r[-2:]
                    for i in range(0, len(r), 2):
                        bx, by = r[i:i + 2]
                        edges.append((ax, ay, bx, by))
                        ax, ay = bx, by
                box = min(ring[::2]), max(ring[::2]), min(ring[1::2]), max(ring[1::2])
                p = (raw['base'], raw['top'], rings, edges, box)
                edge_grid = {}
                for edge in edges:
                    ax, ay, bx, by = edge
                    for ey in range(math.floor(min(ay, by) / self.bucket), math.floor(max(ay, by) / self.bucket) + 1):
                        for ex in range(math.floor(min(ax, bx) / self.bucket), math.floor(max(ax, bx) / self.bucket) + 1):
                            edge_grid.setdefault((ex, ey), []).append(edge)
                self.edge_grids.append(edge_grid)
                for ring_points in rings:
                    rows = {}
                    ax, ay = ring_points[-2:]
                    for i in range(0, len(ring_points), 2):
                        bx, by = ring_points[i:i + 2]
                        if ay != by:
                            for row in range(math.floor(min(ay, by) / 8), math.floor(max(ay, by) / 8) + 1):
                                rows.setdefault(row, []).append((ax, ay, bx, by))
                        ax, ay = bx, by
                    self.ring_rows[id(ring_points)] = rows
                idx = len(self.prisms)
                self.prisms.append(p)
                for iy in range(math.floor(box[2] / self.bucket), math.floor(box[3] / self.bucket) + 1):
                    for ix in range(math.floor(box[0] / self.bucket), math.floor(box[1] / self.bucket) + 1):
                        self.buckets.setdefault((ix, iy), []).append(idx)
            self.valid = True
            self._build_grid()
        except (OSError, ValueError, KeyError, IndexError) as exc:
            print('map unavailable:', type(exc).__name__, file=sys.stderr, flush=True)

    def ground(self, x, y):
        if not self.terrain:
            return 0.
        t = self.t
        u = max(0., min(t['nx'] - 1.001, (x - t['minX']) / t['step']))
        v = max(0., min(t['ny'] - 1.001, (y - t['minY']) / t['step']))
        i, j = int(u), int(v)
        a, b = u - i, v - j
        idx = j * t['nx'] + i
        h00, h10 = self.terrain[idx], self.terrain[idx + 1]
        h01, h11 = self.terrain[idx + t['nx']], self.terrain[idx + t['nx'] + 1]
        return ((h00 * (1 - a) + h10 * a) * (1 - b) + (h01 * (1 - a) + h11 * a) * b) * .01

    def nearby(self, x, y, radius=0.):
        a, b = math.floor((x - radius) / self.bucket), math.floor((x + radius) / self.bucket)
        c, d = math.floor((y - radius) / self.bucket), math.floor((y + radius) / self.bucket)
        if a == b and c == d:
            return self.buckets.get((a, c), ())
        result = set()
        for j in range(c, d + 1):
            for i in range(a, b + 1):
                result.update(self.buckets.get((i, j), ()))
        return result

    def in_prism(self, x, y, rings):
        def contains(ring):
            rows = getattr(self, 'ring_rows', {}).get(id(ring))
            if rows is None:
                return inside(x, y, ring)
            hit = False
            for ax, ay, bx, by in rows.get(math.floor(y / 8), ()):
                if (ay > y) != (by > y) and x < (bx - ax) * (y - ay) / (by - ay) + ax:
                    hit = not hit
            return hit
        return contains(rings[0]) and not any(contains(h) for h in rings[1:])

    def clear_at(self, x, y, z=None, radius=None, height=None):
        r = self.radius if radius is None else radius
        h = self.height if height is None else height
        lo_x, hi_x, lo_y, hi_y = self.bounds
        if not (lo_x + r <= x <= hi_x - r and lo_y + r <= y <= hi_y - r):
            return False
        z = self.ground(x, y) if z is None else z
        for idx in self.nearby(x, y, r):
            base, top, rings, edges, box = self.prisms[idx]
            if top <= z + self.step_up or base >= z + h:
                continue
            if x < box[0] - r or x > box[1] + r or y < box[2] - r or y > box[3] + r:
                continue
            if self.in_prism(x, y, rings):
                return False
            for ey in range(math.floor((y - r) / self.bucket), math.floor((y + r) / self.bucket) + 1):
                for ex in range(math.floor((x - r) / self.bucket), math.floor((x + r) / self.bucket) + 1):
                    for edge in self.edge_grids[idx].get((ex, ey), ()):
                        if segment_dist2(x, y, *edge) < r * r:
                            return False
        return True

    def walk_clear(self, a, b, radius=None, height=None):
        d = math.hypot(b[0] - a[0], b[1] - a[1])
        n = max(1, math.ceil(d / 2.0))
        old_z = a[2] if len(a) > 2 else self.ground(*a[:2])
        # Keep a roof's elevation while the feet are over it, but permit walking off.
        roof = old_z > self.ground(*a[:2]) + self.step_up + .5
        for k in range(1, n + 1):
            u = k / n
            x, y = a[0] + (b[0] - a[0]) * u, a[1] + (b[1] - a[1]) * u
            z = self.ground(x, y)
            if roof:
                for idx in self.nearby(x, y):
                    base, top, rings, edges, box = self.prisms[idx]
                    if abs(top - old_z) < self.step_up + .5 and self.in_prism(x, y, rings):
                        z = max(z, top)
            if z - old_z > self.step_up and (z - old_z) / (d / n + .01) > self.max_slope:
                return False
            if not self.clear_at(x, y, z, radius, height):
                return False
            old_z = z
        return True

    def line_clear(self, a, b):
        if not self.valid:
            return True
        dx, dy, dz = b[0] - a[0], b[1] - a[1], b[2] - a[2]
        low_x, high_x = sorted((a[0], b[0]))
        low_y, high_y = sorted((a[1], b[1]))
        candidates = set()
        # Visit buckets along the ray rather than the whole diagonal bounding rectangle.
        i, j = math.floor(a[0] / self.bucket), math.floor(a[1] / self.bucket)
        end_i, end_j = math.floor(b[0] / self.bucket), math.floor(b[1] / self.bucket)
        candidates.update(self.buckets.get((end_i, end_j), ()))
        sx, sy = (1 if dx >= 0 else -1), (1 if dy >= 0 else -1)
        tx = (((i + (sx > 0)) * self.bucket - a[0]) / dx) if abs(dx) > 1e-12 else math.inf
        ty = (((j + (sy > 0)) * self.bucket - a[1]) / dy) if abs(dy) > 1e-12 else math.inf
        dtx = self.bucket / abs(dx) if abs(dx) > 1e-12 else math.inf
        dty = self.bucket / abs(dy) if abs(dy) > 1e-12 else math.inf
        # Bound traversal even when a sub-picometer boundary crossing rounds a delta to zero.
        for _ in range(abs(end_i - i) + abs(end_j - j) + 4):
            candidates.update(self.buckets.get((i, j), ()))
            if i == end_i and j == end_j:
                break
            if tx < ty:
                i, tx = i + sx, tx + dtx
            else:
                j, ty = j + sy, ty + dty
        for idx in candidates:
            base, top, rings, edges, box = self.prisms[idx]
            if box[1] < low_x or box[0] > high_x or box[3] < low_y or box[2] > high_y:
                continue
            if max(a[2], b[2]) < base or min(a[2], b[2]) > top:
                continue
            if abs(dz) < 1e-9:
                lo, hi = 0., 1.
            else:
                t0, t1 = sorted(((base - a[2]) / dz, (top - a[2]) / dz))
                lo, hi = max(0., t0), min(1., t1)
                if hi < lo:
                    continue
            cuts = [lo, hi]
            for ax, ay, bx, by in edges:
                ex, ey = bx - ax, by - ay
                det = dx * ey - dy * ex
                if abs(det) < 1e-10:
                    continue
                qx, qy = ax - a[0], ay - a[1]
                u = (qx * ey - qy * ex) / det
                v = (qx * dy - qy * dx) / det
                if lo <= u <= hi and 0. <= v <= 1.:
                    cuts.append(u)
            cuts.sort()
            for k in range(len(cuts) - 1):
                u = (cuts[k] + cuts[k + 1]) * .5
                if self.in_prism(a[0] + dx * u, a[1] + dy * u, rings):
                    return False
        n = max(1, math.ceil(math.hypot(dx, dy) / 4.))
        for k in range(1, n):
            u = k / n
            if self.ground(a[0] + dx * u, a[1] + dy * u) > a[2] + dz * u + .15:
                return False
        return True

    def _build_grid(self):
        x0, x1, y0, y1 = self.bounds
        self.x0, self.y0 = x0 + self.cell / 2, y0 + self.cell / 2
        self.nx = int((x1 - x0) / self.cell)
        self.ny = int((y1 - y0) / self.cell)
        self.grid = bytearray(self.nx * self.ny)
        self.heights = array('f', [0.]) * len(self.grid)
        for j in range(self.ny):
            y = self.y0 + j * self.cell
            for i in range(self.nx):
                idx = j * self.nx + i
                x = self.x0 + i * self.cell
                self.heights[idx] = self.ground(x, y)
                self.grid[idx] = self.clear_at(x, y, self.heights[idx])

    def grid_index(self, point):
        i = max(0, min(self.nx - 1, round((point[0] - self.x0) / self.cell)))
        j = max(0, min(self.ny - 1, round((point[1] - self.y0) / self.cell)))
        return j * self.nx + i

    def grid_point(self, idx):
        return self.x0 + (idx % self.nx) * self.cell, self.y0 + (idx // self.nx) * self.cell

    def nearest_open(self, point, reachable=False):
        idx = self.grid_index(point)
        if self.grid[idx] and (not reachable or self.walk_clear(point, self.grid_point(idx), self.radius - .5)):
            return idx
        i, j = idx % self.nx, idx // self.nx
        for radius in range(1, 18):
            options = []
            for dy in range(-radius, radius + 1):
                for dx in range(-radius, radius + 1):
                    if max(abs(dx), abs(dy)) != radius:
                        continue
                    x, y = i + dx, j + dy
                    if 0 <= x < self.nx and 0 <= y < self.ny:
                        k = y * self.nx + x
                        if self.grid[k]:
                            options.append(((self.grid_point(k)[0] - point[0]) ** 2 + (self.grid_point(k)[1] - point[1]) ** 2, k))
            if options:
                options.sort()
                for _, candidate in options:
                    if not reachable or self.walk_clear(point, self.grid_point(candidate), self.radius - .5):
                        return candidate
        return idx

    def path(self, start, goal, limit=2200, deadline=math.inf):
        """A bounded A*: return the best partial path when a long search needs more time."""
        if not self.grid:
            return [tuple(goal[:2])]
        s, g = self.nearest_open(start, reachable=True), self.nearest_open(goal)
        if not self.grid[s] or not self.grid[g]:
            return [tuple(goal[:2])]
        gx, gy = g % self.nx, g // self.nx
        def heuristic(idx):
            dx, dy = abs(idx % self.nx - gx), abs(idx // self.nx - gy)
            return max(dx, dy) + .41421356 * min(dx, dy)
        opened = [(heuristic(s), 0., s)]
        costs, parents = {s: 0.}, {}
        best, best_h = s, heuristic(s)
        steps = 0
        while opened and steps < limit:
            if steps % 32 == 0 and time.perf_counter() > deadline:
                break
            _, cost, idx = heapq.heappop(opened)
            if cost > costs[idx] + .001:
                continue
            steps += 1
            h = heuristic(idx)
            if h < best_h:
                best, best_h = idx, h
            if idx == g:
                best = g
                break
            x, y = idx % self.nx, idx // self.nx
            for dx, dy, weight in ((1, 0, 1), (-1, 0, 1), (0, 1, 1), (0, -1, 1), (1, 1, 1.41421356), (1, -1, 1.41421356), (-1, 1, 1.41421356), (-1, -1, 1.41421356)):
                if not (0 <= x + dx < self.nx and 0 <= y + dy < self.ny):
                    continue
                nxt = idx + dx + dy * self.nx
                if not self.grid[nxt]:
                    continue
                if dx and dy and (not self.grid[idx + dx] or not self.grid[idx + dy * self.nx]):
                    continue
                climb = self.heights[nxt] - self.heights[idx]
                if climb > self.step_up and climb > self.cell * weight * self.max_slope:
                    continue
                new_cost = cost + weight + max(0., climb) * .04
                if new_cost + .001 < costs.get(nxt, 1e30):
                    costs[nxt] = new_cost
                    parents[nxt] = idx
                    heapq.heappush(opened, (new_cost + heuristic(nxt) * 1.15, new_cost, nxt))
        route = [best]
        while route[-1] in parents:
            route.append(parents[route[-1]])
        route.reverse()
        points = [self.grid_point(k) for k in route]
        if best == g and self.clear_at(*goal[:2]):
            points.append(tuple(goal[:2]))
        return points or [tuple(start[:2])]
