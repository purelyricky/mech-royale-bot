package bot

import (
	"apexbot/internal/proto"
	"math"
)

// Beam endpoints supply an observed firing clock and evidence of who is fighting.
func (b *Bot) observeBeams(o proto.Observation) {
	for _, bm := range o.Beams {
		if bm.Kind != "laser" || bm.By == o.Self.ID || len(bm.Path) < 2 {
			continue
		}
		t := b.Tracks[bm.By]
		if t == nil {
			continue
		}
		fired := o.T - math.Max(0, bm.Age)
		if fired <= t.LaserAt+0.06 {
			continue
		}
		t.LaserAt = fired
		end := bm.Path[len(bm.Path)-1]
		if end.XY().Distance(o.Self.Pos.XY()) < 15 {
			continue
		}
		for _, other := range b.Tracks {
			if other.ID == bm.By || o.T-other.Seen > 0.3 {
				continue
			}
			h := b.Mech.Get("height", 9) * b.scale(other.Cubes)
			if end.XY().Distance(other.Pos.XY()) < b.Mech.Get("radius", 2.5)*b.scale(other.Cubes)+1 && end[2] >= other.Pos[2]-1 && end[2] <= other.Pos[2]+h+1 {
				b.ForeignBeams++
				break
			}
		}
	}
	if b.ForeignBeams >= 3 && b.Now <= 180 {
		b.FFA = true
	}
}
