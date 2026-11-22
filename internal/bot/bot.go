// Package bot ports the lab-v8 Apex tactical controller to native Go.
package bot

import (
	"apexbot/internal/proto"
	"apexbot/internal/world"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"
)

type track struct {
	proto.Mech
	Seen, Hit, Dist, Closing float64
	LOS, Engage              bool
}
type known struct {
	proto.Item
	Seen float64
}
type policy struct {
	Ghost, Peek, Hunt                                                                       bool
	LaserRange, LootRadius, LaserSeek, FarmHP, Stick, HPWeight, LowBonus, FightLoot, Finish float64
}
type Bot struct {
	Hello                                                          proto.Hello
	World                                                          *world.World
	RNG                                                            *rand.Rand
	Tracks, NPCs                                                   map[int]*track
	Crates, Cubes, Pickups                                         map[int]*known
	Bad                                                            map[int]float64
	Now                                                            float64
	P                                                              policy
	FFA, Retreat                                                   bool
	Kills, StartAlive                                              int
	LastHurt, HurtBearing, NPCHurt, NPCBearing, LastThreat, LoseAt float64
	StickID                                                        int
	StickAt                                                        float64
	Strafe, StrafeUntil                                            float64
	Path                                                           []proto.Vec2
	PathGoal                                                       proto.Vec2
	PathAt                                                         float64
	PathIndex                                                      int
	LootID                                                         int
	LootSince                                                      float64
	CoverGoal, Home                                                proto.Vec2
	CoverAt, HomeAt                                                float64
	CoverOK, HomeOK                                                bool
	StuckSince, UnstuckUntil                                       float64
	UnstuckDir                                                     proto.Vec2
	Mech, NPC, Crate, Cube, Light, Sound                           proto.Stats
	Started                                                        time.Time
	EscapeLaunch, EscapeLand                                       proto.Vec2
	EscapeAt                                                       float64
	EscapeOK, EscapeFlying                                         bool
}

func New(h proto.Hello) *Bot {
	b := &Bot{Hello: h, RNG: rand.New(rand.NewSource(h.Seed + int64(h.Slot) + 1)), Tracks: map[int]*track{}, NPCs: map[int]*track{}, Crates: map[int]*known{}, Cubes: map[int]*known{}, Pickups: map[int]*known{}, Bad: map[int]float64{}, LastHurt: -100, NPCHurt: -100, LastThreat: -100, LoseAt: -100, StickID: -1, StickAt: -100, Strafe: 1, PathAt: -100, LootID: -1, CoverAt: -100, HomeAt: -100, StuckSince: -1, Mech: h.Group("mech"), NPC: h.Group("npc"), Crate: h.Group("crates"), Cube: h.Group("cubes"), Light: h.Weapon("lightning"), Sound: h.Group("sound")}
	w, err := world.New(h.Map.Dir, b.Mech)
	if err != nil {
		for _, dir := range []string{"arena", "/arena"} {
			if w, err = world.New(dir, b.Mech); err == nil {
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "map unavailable; using geometry-free fallback:", err)
		w = world.Flat()
	}
	b.World = w
	return b
}
func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
func angle(a, b float64) float64      { return math.Remainder(a-b, 2*math.Pi) }
func unit(p proto.Vec2) proto.Vec2 {
	d := math.Hypot(p[0], p[1])
	if d < 1e-8 {
		return proto.Vec2{}
	}
	return proto.Vec2{p[0] / d, p[1] / d}
}
func toward(a, b proto.Vec2) proto.Vec2 { return unit(proto.Vec2{b[0] - a[0], b[1] - a[1]}) }
func movePoint(p, d proto.Vec2, r float64) proto.Vec2 {
	return proto.Vec2{p[0] + d[0]*r, p[1] + d[1]*r}
}
func at(p proto.Vec2, z float64) proto.Vec3 { return proto.Vec3{p[0], p[1], z} }
func (b *Bot) scale(c int) float64          { return 1 + b.Cube.Get("size_per_cube", 0.05)*float64(c) }
func (b *Bot) center(t *track) proto.Vec3 {
	p := t.Pos
	h := b.Mech.Get("height", 9) * b.scale(t.Cubes)
	if t.Kind == "npc" {
		h = b.NPC.Get("height", 3.5)
	}
	p[2] += h * 0.5
	return p
}
func (b *Bot) eye(t *track) proto.Vec3 {
	p := t.Pos
	p[2] += b.Mech.Get("eye_height", 8) * b.scale(t.Cubes)
	return p
}
func (b *Bot) body(me proto.Self) (float64, float64) {
	return me.Stats.Get("radius", b.Mech.Get("radius", 2.5)*b.scale(me.Cubes)), me.Stats.Get("height", b.Mech.Get("height", 9)*b.scale(me.Cubes))
}
func (b *Bot) room() bool { return time.Since(b.Started) < 28*time.Millisecond }
func (b *Bot) ordered(tab map[int]*track) []*track {
	out := make([]*track, 0, len(tab))
	for _, t := range tab {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (b *Bot) items(tab map[int]*known) []*known {
	out := make([]*known, 0, len(tab))
	for _, t := range tab {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (b *Bot) observe(o proto.Observation) {
	b.Now = o.T
	me := o.Self
	p := me.Pos
	origin := p
	origin[2] += me.Stats.Get("muzzle_height", b.Mech.Get("muzzle_height", 7)*b.scale(me.Cubes))
	for _, m := range o.Mechs {
		tab := b.Tracks
		if m.Kind == "npc" {
			tab = b.NPCs
		}
		t := tab[m.ID]
		if t == nil {
			t = &track{Hit: -100}
			tab[m.ID] = t
		}
		t.Mech = m
		if t.MaxHP <= 0 {
			t.MaxHP = 1400
			if m.Kind == "npc" {
				t.MaxHP = 350
			}
		}
		t.Seen = o.T
		t.Dist = p.XY().Distance(m.Pos.XY())
		t.LOS = b.World.Clear(origin, b.center(t))
		if !t.LOS && m.Kind != "npc" {
			top := m.Pos
			top[2] += b.Mech.Get("height", 9) * b.scale(m.Cubes) * 0.85
			t.LOS = b.World.Clear(origin, top)
		}
		if t.Dist > 1 {
			t.Closing = -((m.Vel[0]-me.Vel[0])*(m.Pos[0]-p[0]) + (m.Vel[1]-me.Vel[1])*(m.Pos[1]-p[1])) / t.Dist
		}
	}
	for id, t := range b.Tracks {
		if o.T-t.Seen > 20 {
			delete(b.Tracks, id)
		}
	}
	for id, t := range b.NPCs {
		if o.T-t.Seen > 120 {
			delete(b.NPCs, id)
		}
	}
	eye := p
	eye[2] += me.Stats.Get("eye_height", 8)
	upd := func(tab map[int]*known, items []proto.Item) {
		for _, it := range items {
			tab[it.ID] = &known{it, o.T}
		}
		for id, k := range tab {
			if k.Seen != o.T && o.T-k.Seen > 0.35 && k.Pos.XY().Distance(p.XY()) < 40 {
				tp := k.Pos
				tp[2] += 1.5
				if b.World.Clear(eye, tp) {
					delete(tab, id)
				}
			}
		}
	}
	upd(b.Crates, o.Crates)
	upd(b.Cubes, o.Cubes)
	upd(b.Pickups, o.Pickups)
	for _, e := range o.Events {
		switch e.Type {
		case "damaged":
			b.LastHurt = o.T
			if e.Bearing != nil {
				b.HurtBearing = *e.Bearing
			}
			if e.Weapon == "npc" || e.Weapon == "npc_gun" {
				b.NPCHurt = o.T
				if e.Bearing != nil {
					b.NPCBearing = *e.Bearing
				}
			}
			if e.By != nil {
				if t := b.Tracks[*e.By]; t != nil {
					t.Hit = o.T
				}
				if t := b.NPCs[*e.By]; t != nil {
					t.Hit = o.T
				}
			}
		case "kill":
			b.Kills++
			delete(b.Tracks, e.Target)
			delete(b.NPCs, e.Target)
		}
	}
	if b.StartAlive == 0 {
		b.StartAlive = o.Alive
	}
	// Count foreign deaths, rather than frames after one death (the reference's
	// counter would misclassify a squad match five frames after one early death).
	if b.Now <= 180 && b.StartAlive-o.Alive-b.Kills >= 5 {
		b.FFA = true
	}
	b.P = policy{Ghost: true, Peek: true, LaserRange: 85, LootRadius: 90, FarmHP: 0.70, HPWeight: 60.0 / 1400}
	if o.T > 20 && !b.FFA {
		b.P = policy{Hunt: true, LaserRange: 120, LootRadius: 150, LaserSeek: 240, FarmHP: 0.5, HPWeight: 0.18, Stick: 200, LowBonus: 200, FightLoot: 70, Finish: 20}
	}
	if b.FFA {
		b.P.LaserSeek = 180
		b.P.Stick = 120
		b.P.LowBonus = 120
		b.P.Finish = 24
	}
}

// Forward prediction starts with the live current circle. It never interpolates
// an already interpolated center a second time during the shrink.
func (b *Bot) circle(z proto.Zone, t float64) (proto.Vec2, float64) {
	start := math.Max(b.Now, z.ShrinkStart)
	if t <= start || z.ShrinkEnd <= start {
		return z.Center, z.Radius
	}
	f := clamp((t-start)/(z.ShrinkEnd-start), 0, 1)
	return proto.Vec2{z.Center[0] + (z.NextCenter[0]-z.Center[0])*f, z.Center[1] + (z.NextCenter[1]-z.Center[1])*f}, z.Radius + (z.NextRadius-z.Radius)*f
}
func (b *Bot) deadline(z proto.Zone, p proto.Vec2, pad float64) float64 {
	if p.Distance(z.NextCenter) <= z.NextRadius-pad {
		return 1e9
	}
	if p.Distance(z.Center) > z.Radius-pad {
		return b.Now
	}
	start := math.Max(b.Now, z.ShrinkStart)
	for k := 0; k <= 24; k++ {
		t := start + (z.ShrinkEnd-start)*float64(k)/24
		c, r := b.circle(z, t)
		if p.Distance(c) > r-pad {
			return t
		}
	}
	return z.ShrinkEnd
}
func (b *Bot) zonePenalty(z proto.Zone, p proto.Vec2) float64 {
	d := p.Distance(z.Center) - z.Radius
	v := 0.0
	if d > -5 {
		v = 150 + 5*(d+5)
	}
	dn := p.Distance(z.NextCenter) - z.NextRadius
	if dn > -5 {
		v += clamp(1-(z.ShrinkStart-b.Now)/25, 0.1, 1) * (25 + 0.8*(dn+5))
	}
	return v
}
func (b *Bot) zoneGoal(z proto.Zone, p proto.Vec2) proto.Vec2 {
	if z.NextRadius <= 30 {
		return z.NextCenter
	}
	r := math.Max(0, z.NextRadius-clamp(0.3*z.NextRadius, 3, 30)-5)
	if p.Distance(z.NextCenter) <= r {
		return p
	}
	return movePoint(z.NextCenter, toward(z.NextCenter, p), r)
}

func (b *Bot) travel(me proto.Self, goal proto.Vec2) proto.Vec2 {
	p := me.Pos.XY()
	d := p.Distance(goal)
	if d < 0.8 {
		return proto.Vec2{}
	}
	r, h := b.body(me)
	if d < 45 && b.World.WalkClear(p, goal, me.Pos[2], r, h) {
		return toward(p, goal)
	}
	need := (len(b.Path) == 0 && b.Now-b.PathAt > 0.35) || (goal.Distance(b.PathGoal) > 5 && b.Now-b.PathAt > 0.25) || b.Now-b.PathAt > 2.5
	if need && b.room() {
		b.Path = b.World.Path(p, goal, 7*time.Millisecond)
		b.PathAt = b.Now
		b.PathGoal = goal
		b.PathIndex = 0
	}
	if len(b.Path) > 0 {
		bi := min(b.PathIndex, len(b.Path)-1)
		bd := math.Inf(1)
		for i := bi; i < min(len(b.Path), bi+10); i++ {
			if dd := p.Distance(b.Path[i]); dd < bd {
				bd, bi = dd, i
			}
		}
		b.PathIndex = bi
		if bd <= 14 {
			for i := min(len(b.Path)-1, bi+12); i > bi; i-- {
				if b.World.WalkClear(p, b.Path[i], me.Pos[2], r, h) {
					return toward(p, b.Path[i])
				}
			}
			if bi+1 < len(b.Path) {
				goal = b.Path[bi+1]
			}
		} else {
			b.Path = nil
		}
	}
	// A blocked direct route falls back to a physically clear heading, not a
	// repeated command into a wall while a bounded A* replan is pending.
	dir := toward(p, goal)
	if b.World.WalkClear(p, movePoint(p, dir, 4), me.Pos[2], r, h) {
		return dir
	}
	base := math.Atan2(dir[1], dir[0])
	for _, off := range []float64{0.4, -0.4, 0.8, -0.8, 1.2, -1.2, 1.6, -1.6, math.Pi} {
		a := base + off
		v := proto.Vec2{math.Cos(a), math.Sin(a)}
		if b.World.WalkClear(p, movePoint(p, v, 5), me.Pos[2], r, h) {
			return v
		}
	}
	return proto.Vec2{}
}
func (b *Bot) preferred(me proto.Self, t *track) float64 {
	if t.Kind == "npc" {
		return 58
	}
	_, light := me.Weapon("lightning")
	_, laser := me.Weapon("laser")
	r := 85.0
	if light != nil && me.Energy > 12 {
		r = 28
	} else if laser != nil {
		r = b.P.LaserRange
	}
	if light == nil {
		if t.Has("lightning") {
			r = math.Max(r, b.Light.Get("range", 45)+19)
		}
		if t.Has("mortar") {
			r = math.Max(r, 80)
		}
	}
	return r
}
func (b *Bot) fight(o proto.Observation, t *track, seen []*track, wantLOS float64, finish bool) proto.Vec2 {
	me := o.Self
	p := me.Pos.XY()
	r, h := b.body(me)
	rp := b.preferred(me, t)
	if finish {
		rp = b.P.Finish
	}
	u := toward(p, t.Pos.XY())
	if b.Now >= b.StrafeUntil {
		if b.RNG.Float64() < 0.6 {
			b.Strafe = -b.Strafe
		}
		b.StrafeUntil = b.Now + 0.3 + b.RNG.Float64()*0.7
	}
	side := proto.Vec2{-u[1] * b.Strafe, u[0] * b.Strafe}
	_, light := me.Weapon("lightning")
	mh := me.Stats.Get("muzzle_height", 7*b.scale(me.Cubes))
	best := math.Inf(-1)
	out := proto.Vec2{}
	rings := 1
	if b.P.Peek {
		rings = 2
	}
	for k := -1; k < 16*rings; k++ {
		if k >= 16 && !b.room() {
			break
		}
		dir := proto.Vec2{}
		rr := 0.0
		if k >= 0 {
			a := float64(k%16) * math.Pi / 8
			dir = proto.Vec2{math.Cos(a), math.Sin(a)}
			rr = 6
			if k >= 16 {
				rr = 12
			}
		}
		q := movePoint(p, dir, rr)
		if k >= 0 && !b.World.WalkClear(p, q, me.Pos[2], r, h) {
			continue
		}
		z := b.World.WalkAt(q[0], q[1])
		d := q.Distance(t.Pos.XY())
		s := -1.1*math.Abs(d-rp) - rr*0.4
		los := b.World.Clear(at(q, z+mh), b.center(t))
		if los {
			s += 45 * wantLOS
		}
		for _, e := range seen {
			if e.ID != t.ID && q.Distance(e.Pos.XY()) < 170 && b.World.Clear(b.eye(e), at(q, z+h*0.6)) {
				s -= 30
			}
			if light == nil && e.Has("lightning") {
				dd := q.Distance(e.Pos.XY())
				if dd < b.Light.Get("range", 45)+16 {
					s -= (b.Light.Get("range", 45) + 16 - dd) * 4.2
				}
			}
		}
		s -= b.zonePenalty(o.Zone, q)
		if wantLOS > 0 {
			s += 9 * (dir[0]*side[0] + dir[1]*side[1])
		}
		if k < 0 {
			s -= 8
			if wantLOS < 0 && !los {
				s += 14
			}
		}
		if s > best {
			best, out = s, dir
		}
	}
	return out
}
func (b *Bot) cover(o proto.Observation, seen []*track) (proto.Vec2, bool) {
	if len(seen) == 0 {
		return proto.Vec2{}, false
	}
	me := o.Self
	p := me.Pos.XY()
	_, h := b.body(me)
	threats := append([]*track(nil), seen...)
	sort.Slice(threats, func(i, j int) bool { return threats[i].Dist < threats[j].Dist })
	threats = threats[:min(4, len(threats))]
	score := func(q proto.Vec2) float64 {
		z := b.World.WalkAt(q[0], q[1])
		s := b.zonePenalty(o.Zone, q)
		near := 230.0
		for _, t := range threats {
			near = math.Min(near, q.Distance(t.Pos.XY()))
			if b.World.Clear(b.eye(t), at(q, z+h)) || b.World.Clear(b.eye(t), at(q, z+h*0.5)) {
				s += 180
			}
		}
		return s - math.Min(80, near)*0.25
	}
	if score(p) < 0 {
		return proto.Vec2{}, true
	}
	if b.CoverOK && b.Now-b.CoverAt < 0.4 {
		return b.travel(me, b.CoverGoal), true
	}
	if !b.room() {
		return proto.Vec2{}, false
	}
	b.CoverAt = b.Now
	q, ok := b.World.Cover(p, score, 5*time.Millisecond)
	b.CoverGoal, b.CoverOK = q, ok
	if !ok {
		return proto.Vec2{}, false
	}
	return b.travel(me, q), true
}
func (b *Bot) wantPickup(me proto.Self, kind string) bool {
	if _, w := me.Weapon(kind); w != nil {
		return false
	}
	if kind == "gun" {
		return len(me.Weapons) == 0 || me.Weapons[0] == nil
	}
	rank := func(k string) int {
		switch k {
		case "laser":
			return 4
		case "lightning":
			return 3
		}
		return 0
	}
	cur := 0
	if len(me.Weapons) > 1 && me.Weapons[1] != nil {
		cur = rank(me.Weapons[1].Kind)
	}
	return rank(kind) > cur
}
func (b *Bot) chooseLoot(o proto.Observation, hpf float64, danger bool) *known {
	me := o.Self
	p := me.Pos.XY()
	speed := me.Stats.Get("walk_speed", 11.7)
	var best *known
	bestU := 0.0
	empty := len(me.Weapons) < 2 || me.Weapons[1] == nil
	consider := func(k *known, kind string, value float64) {
		if b.Bad[k.ID] > b.Now {
			return
		}
		d := p.Distance(k.Pos.XY())
		if d > 260 || k.Pos.XY().Distance(o.Zone.Center) > o.Zone.Radius-6 {
			return
		}
		if b.Now+d*1.4/math.Max(1, speed)+5 > b.deadline(o.Zone, k.Pos.XY(), 6) {
			return
		}
		if k.Pos[2]-b.World.Ground(k.Pos[0], k.Pos[1]) > 3.2 || !b.World.Reachable(k.Pos.XY()) {
			return
		}
		u := value / (d + 18)
		if k.ID == b.LootID {
			u *= 1.35
		}
		if u > bestU {
			v := *k
			v.Kind = kind
			best, bestU = &v, u
		}
	}
	for _, k := range b.items(b.Pickups) {
		rad, v := b.P.LootRadius, 60.0
		if empty {
			v = 130
		}
		if k.Kind == "laser" && b.P.LaserSeek > rad {
			rad = b.P.LaserSeek
			v = math.Max(v, 150)
		}
		if p.Distance(k.Pos.XY()) < rad && b.wantPickup(me, k.Kind) {
			consider(k, "pickup", v)
		}
	}
	cubeV := 75.0
	if float64(me.Cubes) >= b.Cube.Get("stat_cap", 10) {
		cubeV = 25
	}
	for _, k := range b.items(b.Cubes) {
		if p.Distance(k.Pos.XY()) < math.Max(60, b.P.LootRadius*0.7) {
			consider(k, "cube", cubeV)
		}
	}
	if hpf > b.P.FarmHP && !danger && (b.P.Hunt || b.Now < 150) {
		for _, k := range b.items(b.Crates) {
			if p.Distance(k.Pos.XY()) < b.P.LootRadius {
				consider(k, "crate", cubeV*0.6)
			}
		}
		if float64(me.Cubes) < b.Cube.Get("stat_cap", 10) {
			for _, n := range b.ordered(b.NPCs) {
				if b.Now-n.Seen < 12 && n.Dist < b.P.LootRadius {
					consider(&known{proto.Item{ID: n.ID, Pos: n.Pos, HP: n.HP}, n.Seen}, "npc", 85)
				}
			}
		}
	}
	if best == nil {
		b.LootID = -1
		return nil
	}
	if best.ID != b.LootID {
		b.LootID, b.LootSince = best.ID, b.Now
	} else if b.Now-b.LootSince > 30 {
		b.Bad[best.ID] = b.Now + 60
		b.LootID = -1
		return nil
	}
	return best
}
func (b *Bot) home(o proto.Observation) proto.Vec2 {
	p := o.Self.Pos.XY()
	out := !b.HomeOK || b.Home.Distance(o.Zone.NextCenter) > o.Zone.NextRadius*0.8
	cost := func(q proto.Vec2) float64 {
		v := 0.25*p.Distance(q) + b.zonePenalty(o.Zone, q)
		for _, t := range b.ordered(b.Tracks) {
			v += math.Max(0, 80-q.Distance(t.Pos.XY())) * 0.5
		}
		z := b.World.WalkAt(q[0], q[1])
		exposure := 0.0
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			d := proto.Vec2{math.Cos(a), math.Sin(a)}
			r := movePoint(q, d, 55)
			if b.World.Clear(at(q, z+8), at(r, b.World.Ground(r[0], r[1])+7)) {
				exposure += 15
			}
		}
		return v + exposure
	}
	if (out || b.Now-b.HomeAt > 12) && b.room() {
		b.HomeAt = b.Now
		best, bv := b.Home, math.Inf(1)
		if b.HomeOK {
			bv = cost(b.Home)
		}
		for k := 0; k < 18; k++ {
			if k >= 6 && !b.room() {
				break
			}
			a := b.RNG.Float64() * 2 * math.Pi
			r := math.Sqrt(b.RNG.Float64()) * o.Zone.NextRadius * 0.7
			q := movePoint(o.Zone.NextCenter, proto.Vec2{math.Cos(a), math.Sin(a)}, r)
			if !b.World.Reachable(q) {
				continue
			}
			if v := cost(q); v < bv {
				best, bv = q, v
			}
		}
		if !math.IsInf(bv, 1) {
			b.Home, b.HomeOK = best, true
		}
	}
	if !b.HomeOK {
		return b.zoneGoal(o.Zone, p)
	}
	return b.Home
}

func (b *Bot) odds(me proto.Self, t *track, opening bool) float64 {
	myW := 0
	for _, w := range me.Weapons {
		if w != nil {
			myW++
		}
	}
	z := 1.5093*math.Log(math.Max(me.HP, 1)/math.Max(t.HP, 1)) + 0.7049*(math.Min(float64(me.Cubes), 10)-math.Min(float64(t.Cubes), 10))/10 + 0.9583*float64(myW-max(1, len(t.Weapons)))
	if opening {
		z += 0.0562 + 0.7475*math.Min(t.Dist, 200)/100
		if math.Abs(angle(t.Aim[0], math.Atan2(me.Pos[1]-t.Pos[1], me.Pos[0]-t.Pos[0]))) > 1.75 {
			z += 0.8235
		}
	}
	return 1 / (1 + math.Exp(-clamp(z, -20, 20)))
}
func (b *Bot) Act(o proto.Observation) proto.Action {
	b.Started = time.Now()
	b.observe(o)
	me := o.Self
	p := me.Pos.XY()
	hpf := me.HP / math.Max(1, me.MaxHP)
	a := proto.Action{Tick: o.Tick, Aim: me.Aim}
	if o.Offer != nil && len(o.Offer.Choices) > 0 {
		i := b.upgrade(o)
		a.Pick = &i
	}
	_, gun := me.Weapon("gun")
	_, laser := me.Weapon("laser")
	_, light := me.Weapon("lightning")
	late := o.Zone.NextRadius <= 70 || o.Alive <= 3
	seen := []*track{}
	var target, hidden *track
	best := math.Inf(1)
	crowd := 0
	danger := false
	for _, t := range b.ordered(b.Tracks) {
		if b.Now-t.Seen > 2.5 {
			continue
		}
		seen = append(seen, t)
		t.Engage = false
		if t.Seen != b.Now {
			t.LOS = false
			continue
		}
		off := math.Abs(angle(t.Aim[0], math.Atan2(p[1]-t.Pos[1], p[0]-t.Pos[0])))
		aimed := off < 0.22
		if b.P.Ghost {
			t.Engage = t.Dist < 60 || b.Now-t.Hit < 4 || aimed && t.Charging && t.Dist < 120 || t.HP < 350 && t.Dist < 120 || o.Alive <= 2
			if b.FFA && !t.Engage {
				t.Engage = hpf > 0.75 && t.HP < 0.55*t.MaxHP && off > 0.6 && t.Dist < 165 || hpf > 0.85 && off > 1.75 && t.Dist < 120 || t.LOS && hpf > 0.5 && t.Dist < 160 && b.odds(me, t, true) >= 0.68
			}
		} else {
			t.Engage = t.Dist < 60 || b.Now-t.Hit < 6 || late || t.Closing > 3 && t.Dist < 180 || t.HP < 0.4*t.MaxHP && t.Dist < 160 || aimed && t.Dist < 190
		}
		if t.Engage || b.Now-t.Hit < 4 {
			danger = true
		}
		if t.Engage || t.LOS && t.Dist < 150 {
			crowd++
		}
		if t.Engage && !t.LOS && t.Dist < 215 && (hidden == nil || t.Dist < hidden.Dist) {
			hidden = t
		}
		if !t.Engage || !t.LOS || t.Dist > 215 {
			continue
		}
		s := t.Dist + b.P.HPWeight*t.HP
		if t.HP < 0.3*t.MaxHP {
			s -= b.P.LowBonus
		}
		if t.ID == b.StickID && b.Now-b.StickAt < 2 {
			s -= b.P.Stick
		}
		if b.Now-t.Hit < 4 {
			s -= 40
		}
		if t.Closing > 3 {
			s -= 15
		}
		if s < best {
			best, target = s, t
		}
	}
	if target != nil {
		b.StickID, b.StickAt = target.ID, b.Now
	}
	foe := target
	if foe == nil && b.P.Peek {
		foe = hidden
	}
	if danger || b.Now-b.LastHurt < 3 {
		b.LastThreat = b.Now
	}
	finishing := target != nil && target.Dist < 120 && (target.HP < 200 || target.HP < me.HP*0.5)
	outgunned := target != nil && target.Has("laser") && laser == nil && target.Dist > 55 && hpf < 0.8
	swamped := crowd >= 3 || crowd == 2 && hpf < 0.65 || outgunned
	if b.FFA && target != nil && !finishing && b.odds(me, target, false) < 0.3 {
		swamped = true
	}
	if !b.Retreat && (hpf < 0.38 && b.Now-b.LastThreat < 2 || swamped) {
		b.Retreat = true
		b.CoverAt = -100
	}
	if b.Retreat && !swamped && (hpf > 0.8 || b.Now-b.LastThreat > 7 && hpf > 0.55) {
		b.Retreat = false
	}
	if b.P.Hunt {
		near := 0
		for _, t := range seen {
			if t.Seen == b.Now && t.Dist < 160 {
				near++
			}
		}
		losing := near >= 2 && hpf < 0.55 || near >= 3 && hpf < 0.8
		if target != nil {
			tf := target.HP / math.Max(1, target.MaxHP)
			if hpf < 0.35 && tf > hpf+0.1 {
				losing = true
			}
			if tf < 0.25 && hpf > 0.2 && near <= 2 {
				losing = false
			}
		}
		if losing {
			b.LoseAt = b.Now
		}
		b.Retreat = b.Now-b.LoseAt < 1.5 && b.LoseAt > 0
	}
	var closeLight *track
	if light == nil {
		for _, t := range seen {
			if t.Seen == b.Now && t.Has("lightning") && t.Dist < b.Light.Get("range", 45)+6 && (closeLight == nil || t.Dist < closeLight.Dist) {
				closeLight = t
			}
		}
	}
	goal := b.zoneGoal(o.Zone, p)
	speed := math.Max(1, me.Stats.Get("walk_speed", 11.7))
	need := p.Distance(goal)*1.45/speed + 4
	deadline := b.deadline(o.Zone, p, 8)
	must := !me.InZone || b.Now+need+3 >= deadline
	should := b.Now+need+14 >= deadline
	var farm *track
	var farmPos *proto.Vec3
	mode := "idle"
	escapeMove, escapeBoost, escapeJump, escaping := b.escape(me)
	switch {
	case escaping:
		a.Move, a.Boost, a.Jump = escapeMove, escapeBoost, escapeJump
		mode = "escape"
	case b.Now < b.UnstuckUntil:
		a.Move = b.UnstuckDir
		mode = "unstuck"
	case must && p.Distance(goal) > 2:
		a.Move = b.travel(me, goal)
		a.Boost = (!me.InZone || b.Now+need >= deadline) && me.Energy > 5
		mode = "zone"
	case closeLight != nil && !finishing:
		a.Move = b.kite(o, closeLight, seen)
		a.Boost = closeLight.Dist < b.Light.Get("range", 45)+1 && me.Energy > 15
		mode = "kite"
	case b.Retreat && !finishing && !late:
		if v, ok := b.cover(o, seen); ok {
			a.Move = v
			a.Boost = me.Energy > 10
			mode = "cover"
		} else if target != nil {
			a.Move = b.fight(o, target, seen, 1, false)
			mode = "cornered"
		} else {
			a.Move = b.travel(me, goal)
			mode = "flee"
		}
	case target == nil && b.Now-b.LastHurt < 2.5 && !late && b.P.Ghost:
		t := &track{Mech: proto.Mech{ID: -999, Pos: at(movePoint(p, proto.Vec2{math.Cos(b.HurtBearing), math.Sin(b.HurtBearing)}, 80), me.Pos[2])}, Dist: 80}
		if v, ok := b.cover(o, append([]*track{t}, seen...)); ok {
			a.Move = v
			a.Boost = me.Energy > 30
			mode = "cover"
		}
	case foe != nil:
		ready := gun != nil && gun.Ammo > 0 && gun.Reload <= 0.25 && foe.Dist <= gun.Stats.Get("range", 230)-10
		hot := laser != nil && laser.Charge >= laser.Stats.Get("charge", 0.75)-0.3 && foe.Dist <= laser.Stats.Get("range", 280)-5
		if hot || light != nil && foe.Dist <= light.Stats.Get("range", 45)+6 && me.Energy >= light.Stats.Get("energy", 3)*3 {
			ready = true
		}
		if !b.P.Peek && (light != nil || laser != nil) {
			ready = true
		}
		want := -1.0
		if ready {
			want = 1
		}
		if b.P.Peek && !hot {
			for _, t := range seen {
				if t.Charging && t.LOS && math.Abs(angle(t.Aim[0], math.Atan2(p[1]-t.Pos[1], p[0]-t.Pos[0]))) < 0.28 {
					want = -1.6
					break
				}
			}
		}
		finish := b.P.Finish > 0 && foe.HP < 0.38*foe.MaxHP && hpf > 0.42 && crowd <= 2
		a.Move = b.fight(o, foe, seen, want, finish)
		if light != nil && foe.Dist > 50 && me.Energy > 55 && want > 0 {
			a.Boost = true
		}
		mode = "fight"
		if b.P.FightLoot > 0 {
			rad := b.P.FightLoot
			if hpf <= 0.5 {
				rad *= 0.57
			}
			var cb *known
			for _, c := range b.items(b.Cubes) {
				d := p.Distance(c.Pos.XY())
				if math.Abs(c.Pos[2]-me.Pos[2]) > 3.6 || d >= rad {
					continue
				}
				safe := true
				if light == nil {
					for _, t := range seen {
						if t.Has("lightning") && t.Pos.XY().Distance(c.Pos.XY()) < b.Light.Get("range", 45)+4 {
							safe = false
							break
						}
					}
				}
				if safe {
					rad, cb = d, c
				}
			}
			if cb != nil {
				a.Move = b.travel(me, cb.Pos.XY())
				mode = "fight:cube"
			}
		}
	case should && p.Distance(goal) > 2:
		a.Move = b.travel(me, goal)
		mode = "zone"
	default:
		loot := b.chooseLoot(o, hpf, danger)
		if loot != nil {
			mode = "loot"
			a.Move = b.travel(me, loot.Pos.XY())
			if loot.Kind == "npc" {
				n := b.NPCs[loot.ID]
				if n != nil && b.Now-n.Seen < 1 && n.LOS && n.Dist < 75 {
					farm = n
					a.Move = b.fight(o, n, seen, 1, false)
				}
			}
			if loot.Kind == "crate" {
				cp := loot.Pos
				cp[2] += b.Crate.Get("height", 2.4) * 0.5
				mp := me.Pos
				mp[2] += me.Stats.Get("muzzle_height", 7*b.scale(me.Cubes))
				d := p.Distance(loot.Pos.XY())
				if d < 50 && b.World.Clear(mp, cp) {
					farmPos = &cp
					a.Move = proto.Vec2{}
					if d < 12 {
						a.Move = toward(loot.Pos.XY(), p)
					}
				}
			}
		} else if p.Distance(goal) > 3 && (!b.P.Ghost || o.Zone.NextRadius <= 30) {
			a.Move = b.travel(me, goal)
			mode = "zone"
		} else if b.P.Ghost {
			home := b.home(o)
			a.Move = b.travel(me, home)
			if !danger && b.Now-b.LastHurt > 6 {
				q := clamp((b.Sound.Get("step_speed", 5)-0.4)/speed, 0.2, 1)
				a.Move[0] *= q
				a.Move[1] *= q
			}
			mode = "home"
		}
	}
	if mode != "unstuck" && mode != "escape" {
		if v, laserThreat, ok := b.dodge(o); ok {
			v = proto.Vec2{a.Move[0]*0.4 + v[0]*0.9, a.Move[1]*0.4 + v[1]*0.9}
			if math.Hypot(v[0], v[1]) > 1 {
				v = unit(v)
			}
			r, h := b.body(me)
			if b.World.WalkClear(p, movePoint(p, v, 4), me.Pos[2], r, h) {
				a.Move = v
				if laserThreat && me.Energy > 25 {
					a.Boost = true
				}
			}
		}
	}
	if mode == "escape" {
		b.StuckSince = -1
	} else if math.Hypot(a.Move[0], a.Move[1]) > 0.1 && math.Hypot(me.Vel[0], me.Vel[1]) < 1.2 && me.Grounded {
		if b.StuckSince < 0 {
			b.StuckSince = b.Now
		} else if b.Now-b.StuckSince > 0.7 {
			ang := b.RNG.Float64() * 2 * math.Pi
			b.UnstuckDir = proto.Vec2{math.Cos(ang), math.Sin(ang)}
			b.UnstuckUntil = b.Now + 0.5
			b.StuckSince = -1
			b.Path = nil
			a.Jump = true
		}
	} else {
		b.StuckSince = -1
	}
	b.fire(o, &a, target, foe, farm, farmPos, seen)
	if gun != nil && gun.Reload <= 0 && gun.Ammo < int(gun.Stats.Get("mag", 12)) && target == nil && farm == nil && farmPos == nil {
		if float64(gun.Ammo) <= gun.Stats.Get("mag", 12)*0.6 || b.Now-b.LastThreat > 3 {
			a.Reload = true
		}
	}
	b.pickup(o, &a)
	a.Boost = a.Boost && math.Hypot(a.Move[0], a.Move[1]) > 0.01
	a.Aim[0] = angle(a.Aim[0], 0)
	a.Aim[1] = clamp(a.Aim[1], me.Stats.Get("min_pitch", b.Mech.Get("min_pitch", -0.7)), me.Stats.Get("max_pitch", b.Mech.Get("max_pitch", 1.3)))
	return a
}

func (b *Bot) kite(o proto.Observation, t *track, seen []*track) proto.Vec2 {
	me := o.Self
	p := me.Pos.XY()
	base := math.Atan2(p[1]-t.Pos[1], p[0]-t.Pos[0])
	best := math.Inf(-1)
	out := proto.Vec2{}
	r, h := b.body(me)
	for _, rot := range []float64{0, 0.45, -0.45, 0.9, -0.9, 1.35, -1.35} {
		a := base + rot
		v := proto.Vec2{math.Cos(a), math.Sin(a)}
		q := movePoint(p, v, 8)
		if !b.World.WalkClear(p, q, me.Pos[2], r, h) {
			continue
		}
		s := 25*math.Cos(rot) - b.zonePenalty(o.Zone, q)
		for _, e := range seen {
			if e.ID != t.ID && e.Dist < 170 && b.World.Clear(b.eye(e), at(q, b.World.WalkAt(q[0], q[1])+h*0.6)) {
				s -= 18
				if e.Charging {
					s -= 17
				}
			}
		}
		if s > best {
			best, out = s, v
		}
	}
	return out
}
func (b *Bot) dodge(o proto.Observation) (proto.Vec2, bool, bool) {
	me := o.Self
	r, h := b.body(me)
	center := me.Pos
	center[2] += h * 0.56
	out := proto.Vec2{}
	laserThreat := false
	for _, pr := range o.Projectiles {
		if pr.Kind == "mortar" { // Predict the shell's impact against the map, then leave its blast.
			g := b.Hello.Weapon("mortar").Get("gravity", 22)
			for t := 0.1; t <= 1.2; t += 0.1 {
				q := proto.Vec3{pr.Pos[0] + pr.Vel[0]*t, pr.Pos[1] + pr.Vel[1]*t, pr.Pos[2] + pr.Vel[2]*t - 0.5*g*t*t}
				if q[2] <= b.World.Ground(q[0], q[1])+1 {
					if q.XY().Distance(me.Pos.XY()) < b.Hello.Weapon("mortar").Get("splash_radius", 7)+r+3 {
						v := toward(q.XY(), me.Pos.XY())
						if v == (proto.Vec2{}) {
							v = proto.Vec2{b.Strafe, 0}
						}
						out[0] += v[0] * 2
						out[1] += v[1] * 2
					}
					break
				}
			}
			continue
		}
		v := pr.Vel
		v2 := v[0]*v[0] + v[1]*v[1] + v[2]*v[2]
		if v2 < 1 {
			continue
		}
		t := ((center[0]-pr.Pos[0])*v[0] + (center[1]-pr.Pos[1])*v[1] + (center[2]-pr.Pos[2])*v[2]) / v2
		if t <= 0 || t > 0.7 {
			continue
		}
		dx, dy := pr.Pos[0]+v[0]*t-center[0], pr.Pos[1]+v[1]*t-center[1]
		z := pr.Pos[2] + v[2]*t
		if z < me.Pos[2]-1 || z > me.Pos[2]+h+1 || math.Hypot(dx, dy) > r+1.1 {
			continue
		}
		perp := unit(proto.Vec2{-v[1], v[0]})
		if perp[0]*dx+perp[1]*dy > 0 {
			perp[0] = -perp[0]
			perp[1] = -perp[1]
		}
		out[0] += perp[0]
		out[1] += perp[1]
	}
	for _, m := range o.Mechs {
		if m.Kind == "npc" || !m.Charging {
			continue
		}
		to := math.Atan2(me.Pos[1]-m.Pos[1], me.Pos[0]-m.Pos[0])
		if math.Abs(angle(m.Aim[0], to)) < 0.15 {
			a := to + math.Pi*0.5*b.Strafe
			out[0] += math.Cos(a) * 1.5
			out[1] += math.Sin(a) * 1.5
			laserThreat = true
		}
	}
	return unit(out), laserThreat, math.Hypot(out[0], out[1]) > 0.01
}
func (b *Bot) upgrade(o proto.Observation) int {
	order := []string{"plating", "armor", "gun_damage", "nanites", "gun_rate", "lightning_efficiency", "lightning_range", "laser_damage", "servos", "laser_charge", "gun_mag", "gun_velocity", "gyros", "lightning_chain", "reactor", "capacitor", "mortar_reload", "mortar_splash", "mortar_velocity", "sensors", "jets", "laser_pierce"}
	if b.P.Hunt {
		order = []string{"plating", "armor", "laser_damage", "laser_charge", "nanites", "gun_damage", "gun_rate", "lightning_efficiency", "lightning_range", "servos", "gun_mag", "gun_velocity", "gyros", "lightning_chain", "reactor", "capacitor", "mortar_reload", "mortar_splash", "mortar_velocity", "sensors", "jets", "laser_pierce"}
	}
	best, rank := 0, len(order)+1
	for i, c := range o.Offer.Choices {
		r := len(order)
		for j, id := range order {
			if id == c.ID {
				r = j
				break
			}
		}
		if r < rank {
			best, rank = i, r
		}
	}
	return best
}
func (b *Bot) pickup(o proto.Observation, a *proto.Action) {
	me := o.Self
	for _, pk := range o.Pickups {
		if me.Pos.XY().Distance(pk.Pos.XY()) > b.Hello.Group("loot").Get("pickup_radius", 4)-0.3 || math.Abs(pk.Pos[2]-me.Pos[2]) > 3.6 || !b.wantPickup(me, pk.Kind) {
			continue
		}
		if len(me.Weapons) > 1 && me.Weapons[1] != nil && me.Weapons[0] != nil {
			if me.Weapons[1].Kind == "laser" && me.Weapons[1].Charge > 0 {
				continue
			}
			drop := 1
			a.Drop = &drop
		}
		id := pk.ID
		a.Take = &id
		return
	}
}
