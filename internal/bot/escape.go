package bot

import (
	"apexbot/internal/proto"
	"math"
)

func (b *Bot) escape(me proto.Self) (proto.Vec2, bool, bool, bool) {
	p := me.Pos.XY()
	w := b.World
	if !w.Ready {
		return proto.Vec2{}, false, false, false
	}
	jump := false
	if me.Grounded {
		if me.Pos[2]-w.Ground(p[0], p[1]) > 3 || w.Reachable(p) {
			b.EscapeOK, b.EscapeFlying = false, false
			return proto.Vec2{}, false, false, false
		}
		b.EscapeFlying = false
		if !b.EscapeOK || b.Now-b.EscapeAt > 6 {
			b.EscapeLaunch, b.EscapeLand, b.EscapeOK = w.Escape(p)
			b.EscapeAt = b.Now
		}
		if !b.EscapeOK {
			return proto.Vec2{}, false, false, false
		}
		if p.Distance(b.EscapeLaunch) > 3 {
			return b.travel(me, b.EscapeLaunch), false, false, true
		}
		maxEnergy := me.MaxEnergy
		if maxEnergy <= 0 {
			maxEnergy = 100
		}
		if me.Energy < math.Min(70, maxEnergy*0.7) {
			return proto.Vec2{}, false, false, true
		}
		b.EscapeFlying = true
		jump = true
	} else if !b.EscapeFlying {
		return proto.Vec2{}, false, false, false
	}
	if p.Distance(b.EscapeLand) < 1.5 {
		return proto.Vec2{}, false, jump, true
	}
	dir := toward(p, b.EscapeLand)
	need := w.TopAlong(p, movePoint(p, dir, math.Min(p.Distance(b.EscapeLand), 30)))
	return dir, me.Pos[2] < need+2.5 && me.Energy > 1, jump, true
}
