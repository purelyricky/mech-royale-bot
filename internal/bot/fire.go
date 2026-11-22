package bot

import (
	"apexbot/internal/proto"
	"math"
)

const aimLatency = 0.075

func lead(muzzle, tp, tv proto.Vec3, speed float64, grounded bool) (proto.Vec2, proto.Vec3) {
	if speed <= 0 {
		speed = 252
	}
	t := 0.0
	var q proto.Vec3
	for i := 0; i < 4; i++ {
		tt := aimLatency + t
		q = proto.Vec3{tp[0] + tv[0]*tt, tp[1] + tv[1]*tt, tp[2]}
		if !grounded {
			q[2] += tv[2] * tt * 0.5
		}
		dx, dy, dz := q[0]-muzzle[0], q[1]-muzzle[1], q[2]-muzzle[2]
		t = math.Max(0, math.Sqrt(dx*dx+dy*dy+dz*dz)-3) / speed
	}
	return bearing(muzzle, q), q
}
func bearing(a, b proto.Vec3) proto.Vec2 {
	return proto.Vec2{math.Atan2(b[1]-a[1], b[0]-a[0]), math.Atan2(b[2]-a[2], math.Hypot(b[0]-a[0], b[1]-a[1]))}
}
func (b *Bot) fire(o proto.Observation, a *proto.Action, target, foe, farm *track, farmPos *proto.Vec3, seen []*track) {
	me := o.Self
	_, gun := me.Weapon("gun")
	muzzle := me.Pos
	muzzle[0] += me.Vel[0] * aimLatency
	muzzle[1] += me.Vel[1] * aimLatency
	muzzle[2] += me.Stats.Get("muzzle_height", b.Mech.Get("muzzle_height", 7)*b.scale(me.Cubes))
	defend := false
	if target == nil && b.Now-b.NPCHurt < 3 {
		bestOff := 0.7
		for _, n := range b.ordered(b.NPCs) {
			if b.Now-n.Seen > 0.5 || !n.LOS || n.Dist > 150 {
				continue
			}
			off := math.Abs(angle(math.Atan2(n.Pos[1]-me.Pos[1], n.Pos[0]-me.Pos[0]), b.NPCBearing))
			if b.Now-n.Hit < 3 {
				off = 0
			}
			if off < bestOff {
				bestOff, farm = off, n
				farmPos = nil
				defend = true
			}
		}
	}
	var tp, tv proto.Vec3
	tr, th := 1.0, 1.0
	shootable, isMech := false, false
	if target != nil {
		sc := b.scale(target.Cubes)
		tr, th = b.Mech.Get("radius", 2.5)*sc, b.Mech.Get("height", 9)*sc
		tp = b.center(target)
		tv = target.Vel
		if !b.World.Clear(muzzle, tp) {
			tp = target.Pos
			tp[2] += th * 0.85
		}
		shootable, isMech = true, true
	} else if farm != nil {
		tr, th = b.NPC.Get("radius", 1.8), b.NPC.Get("height", 3.5)
		tp = b.center(farm)
		tv = farm.Vel
		shootable = true
	} else if farmPos != nil {
		tr, th = b.Crate.Get("radius", 1.4), b.Crate.Get("height", 2.4)
		tp = *farmPos
		shootable = true
	}
	if !isMech && !defend && (b.Retreat || me.HP/math.Max(1, me.MaxHP) < 0.5) {
		shootable = false
	}
	haveAim := false
	if shootable {
		dist := me.Pos.XY().Distance(tp.XY())
		tickS := 1 / math.Max(1, b.Hello.Group("match").Get("tick_hz", 20))
		hyPoint := tp
		hyPoint[0] += tv[0] * (aimLatency + tickS)
		hyPoint[1] += tv[1] * (aimLatency + tickS)
		hitAim := bearing(muzzle, hyPoint)
		gunAim := hitAim
		gunOK := false
		if gun != nil {
			var q proto.Vec3
			gunAim, q = lead(muzzle, tp, tv, gun.Stats.Get("speed", 252), math.Abs(tv[2]) < 0.5)
			gunOK = dist <= gun.Stats.Get("range", 230)*0.96 && b.World.Clear(muzzle, q)
		}
		a.Aim = hitAim
		if gun != nil {
			a.Aim = gunAim
		}
		haveAim = true
		tolY := math.Atan2(tr*0.75, math.Max(1, dist))
		tolP := math.Atan2(th*0.4, math.Max(1, dist))
		turn := me.Stats.Get("turn_rate", b.Mech.Get("turn_rate", 4)) * tickS
		pitch := me.Stats.Get("pitch_rate", b.Mech.Get("pitch_rate", 3)) * tickS
		slew := func(aim proto.Vec2) bool {
			return math.Max(0, math.Abs(angle(aim[0], me.Aim[0]))-turn) < tolY && math.Max(0, math.Abs(aim[1]-me.Aim[1])-pitch) < tolP
		}
		for i, w := range me.Weapons {
			if i >= 2 {
				break
			}
			if w == nil {
				continue
			}
			switch w.Kind {
			case "gun":
				a.Fire[i] = gunOK && w.Ammo > 0 && w.Reload <= 0 && slew(gunAim)
			case "lightning":
				cone := w.Stats.Get("cone_deg", 30) * math.Pi / 180
				a.Fire[i] = dist < w.Stats.Get("range", 45)-1.5 && math.Abs(angle(hitAim[0], me.Aim[0])) < cone*0.5-0.05 && me.Energy >= w.Stats.Get("energy", 3)
			case "laser":
				if !isMech || dist > w.Stats.Get("range", 280)*0.97 {
					continue
				}
				full := w.Stats.Get("charge", 0.75)
				charge := w.Charge
				minCharge := w.Stats.Get("min_charge", 0.15)
				aligned := math.Abs(angle(hitAim[0], me.Aim[0])) < tolY*0.9 && math.Abs(hitAim[1]-me.Aim[1]) < tolP
				lethal := false
				if charge >= minCharge && charge < full-0.051 {
					minFrac := w.Stats.Get("min_frac", 0.35)
					damage := w.Stats.Get("damage", 315) * (minFrac + (1-minFrac)*charge/math.Max(0.01, full)) * (1 + b.Cube.Get("damage_per_cube", 0.04)*math.Min(float64(me.Cubes), b.Cube.Get("stat_cap", 10)))
					if target.Has("mortar") {
						damage *= 1 + w.Stats.Get("cookoff", 0.6)
					}
					k := 0.8
					if b.P.Hunt {
						k = 0.7
					}
					lethal = damage*k >= target.HP && aligned
				}
				if lethal {
					a.Aim = hitAim
					a.Fire[i] = false
				} else if charge < full-0.051 {
					a.Fire[i] = true
					if charge > full*0.45 {
						a.Aim = hitAim
					}
				} else {
					a.Aim = hitAim
					a.Fire[i] = !aligned
				}
			}
		}
		// Both weapons share the same physical torso. A hitscan takeover must
		// not also fire a gun along a direction that no longer leads the target.
		if gi, _ := me.Weapon("gun"); gi >= 0 && gi < 2 && a.Fire[gi] {
			if math.Abs(angle(a.Aim[0], gunAim[0])) > tolY || math.Abs(a.Aim[1]-gunAim[1]) > tolP {
				a.Fire[gi] = false
			}
		}
	}
	if target == nil && foe != nil {
		for i, w := range me.Weapons {
			if i < 2 && w != nil && w.Kind == "laser" && w.Cooldown <= 0.05 && foe.Dist < w.Stats.Get("range", 280) && w.Charge < w.Stats.Get("charge", 0.75)+w.Stats.Get("hold_s", 2)-0.35 {
				a.Fire[i] = true
			}
		}
	}
	if !haveAim {
		var watch *track
		for _, t := range seen {
			if (t.Engage || b.Now-t.Hit < 6) && (watch == nil || t.Dist < watch.Dist) {
				watch = t
			}
		}
		switch {
		case watch != nil:
			a.Aim = proto.Vec2{math.Atan2(watch.Pos[1]-me.Pos[1], watch.Pos[0]-me.Pos[0]), 0}
		case b.Now-b.NPCHurt < 2.5:
			a.Aim = proto.Vec2{b.NPCBearing, 0}
		case b.Now-b.LastHurt < 2.5:
			a.Aim = proto.Vec2{b.HurtBearing, 0}
		case math.Hypot(a.Move[0], a.Move[1]) > 0:
			a.Aim = proto.Vec2{math.Atan2(a.Move[1], a.Move[0]) + 1.15*math.Sin(b.Now*1.3), 0}
		default:
			a.Aim = proto.Vec2{me.Aim[0] + 0.35, 0}
		}
	}
}
