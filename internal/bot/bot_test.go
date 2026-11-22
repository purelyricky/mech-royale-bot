package bot

import (
	"apexbot/internal/proto"
	"apexbot/internal/world"
	"math"
	"os"
	"sort"
	"testing"
	"time"
)

func fixture() (*Bot, proto.Observation) {
	b := New(proto.Hello{Protocol: "mechfight/1", Seed: 17})
	b.World = world.Flat()
	o := proto.Observation{Type: "obs", Tick: 300, T: 30, Alive: 16, Self: proto.Self{ID: 0, HP: 1400, MaxHP: 1400, Energy: 100, MaxEnergy: 100, Grounded: true, InZone: true, Stats: proto.Stats{"radius": 2.5, "height": 9, "muzzle_height": 7, "turn_rate": 4, "pitch_rate": 3}, Weapons: []*proto.Weapon{{Kind: "gun", Ammo: 12, Stats: proto.Stats{"speed": 252, "range": 230, "mag": 12}}, nil}}, Zone: proto.Zone{Radius: 560, NextRadius: 380, ShrinkStart: 60, ShrinkEnd: 110}}
	return b, o
}
func enemy(x, y float64) proto.Mech {
	return proto.Mech{ID: 1, Kind: "mech", Pos: proto.Vec3{x, y, 0}, HP: 1000, MaxHP: 1400, Aim: proto.Vec2{math.Pi, 0}, Weapons: []string{"gun"}}
}

func TestLeadMovingTarget(t *testing.T) {
	aim, q := lead(proto.Vec3{0, 0, 7}, proto.Vec3{100, 0, 4.5}, proto.Vec3{0, 20, 0}, 252, true)
	if aim[0] < 0.06 || q[1] < 8 {
		t.Fatalf("does not lead lateral motion: %v %v", aim, q)
	}
}
func TestZoneMovementWinsOverCombat(t *testing.T) {
	b, o := fixture()
	o.Self.Pos = proto.Vec3{600, 0, 0}
	o.Self.InZone = false
	o.Mechs = []proto.Mech{enemy(610, 0)}
	a := b.Act(o)
	if a.Move[0] >= 0 || !a.Boost {
		t.Fatalf("must escape zone west: %+v", a)
	}
}
func TestForeignDeathsAreNotFrames(t *testing.T) {
	b, o := fixture()
	b.Act(o)
	for i := 0; i < 10; i++ {
		o.T += 0.1
		o.Alive = 15
		b.Act(o)
	}
	if b.FFA {
		t.Fatal("one death repeated across frames classified as FFA")
	}
	o.Alive = 11
	b.Act(o)
	if !b.FFA {
		t.Fatal("five foreign deaths should switch to FFA")
	}
}

func TestLateZoneDeathsDoNotTriggerFFA(t *testing.T) {
	b, o := fixture()
	b.Act(o)
	o.T = 240
	o.Alive = 8
	b.Act(o)
	if b.FFA {
		t.Fatal("late zone deaths classified squad match as FFA")
	}
}
func TestDirectMortarImpactHasEscapeDirection(t *testing.T) {
	b, o := fixture()
	o.Projectiles = []proto.Projectile{{Kind: "mortar", Pos: proto.Vec3{0, 0, 3}, Vel: proto.Vec3{0, 0, -20}}}
	v, _, ok := b.dodge(o)
	if !ok || math.Hypot(v[0], v[1]) < 0.5 {
		t.Fatal("direct incoming shell produced no dodge")
	}
}
func TestOwnKillsDoNotTriggerFFA(t *testing.T) {
	b, o := fixture()
	b.Act(o)
	o.Alive = 11
	for i := 0; i < 5; i++ {
		o.Events = append(o.Events, proto.Event{Type: "kill", Target: i + 1})
	}
	b.Act(o)
	if b.FFA {
		t.Fatal("our own kills classified as foreign deaths")
	}
}
func TestPhaseTransition(t *testing.T) {
	b, o := fixture()
	o.T = 10
	b.Act(o)
	if !b.P.Ghost || b.P.Hunt {
		t.Fatal("opening should be quiet")
	}
	o.T = 30
	b.Act(o)
	if !b.P.Hunt || b.P.Ghost {
		t.Fatal("hunt phase not activated")
	}
}
func TestLightningKite(t *testing.T) {
	b, o := fixture()
	m := enemy(30, 0)
	m.Weapons = append(m.Weapons, "lightning")
	o.Mechs = []proto.Mech{m}
	a := b.Act(o)
	if a.Move[0] >= 0 {
		t.Fatalf("must flee lightning range: %v", a.Move)
	}
}
func TestLaserChargeAndRelease(t *testing.T) {
	b, o := fixture()
	o.Mechs = []proto.Mech{enemy(80, 0)}
	o.Self.Weapons[1] = &proto.Weapon{Kind: "laser", Stats: proto.Stats{"charge": 0.75, "min_charge": 0.15, "range": 280}}
	o.Self.Aim = bearing(proto.Vec3{0, 0, 7}, proto.Vec3{80, 0, 4.5})
	a := b.Act(o)
	if !a.Fire[1] {
		t.Fatal("laser should charge on target")
	}
	o.Self.Weapons[1].Charge = 0.75
	o.T += 0.1
	a = b.Act(o)
	if a.Fire[1] {
		t.Fatal("aligned full laser should release")
	}
	o.Self.Aim = proto.Vec2{1, 0}
	o.T += 0.1
	a = b.Act(o)
	if !a.Fire[1] {
		t.Fatal("misaligned laser should hold until aimed")
	}
}
func TestPickupRetainsChargingLaser(t *testing.T) {
	b, o := fixture()
	o.Self.Weapons[1] = &proto.Weapon{Kind: "laser", Charge: 0.5}
	o.Pickups = []proto.Item{{ID: 8001, Kind: "lightning", Pos: proto.Vec3{1, 0, 0}}}
	a := b.Act(o)
	if a.Take != nil || a.Drop != nil {
		t.Fatal("must not swap charging laser")
	}
}
func TestLaserPickupReplacesLightning(t *testing.T) {
	b, o := fixture()
	o.Self.Weapons[1] = &proto.Weapon{Kind: "lightning"}
	o.Pickups = []proto.Item{{ID: 8001, Kind: "laser", Pos: proto.Vec3{1, 0, 0}}}
	a := b.Act(o)
	if a.Take == nil || *a.Take != 8001 || a.Drop == nil || *a.Drop != 1 {
		t.Fatalf("expected explicit weapon swap: %+v", a)
	}
}
func TestRooftopLootNotGroundGoal(t *testing.T) {
	b, o := fixture()
	o.Pickups = []proto.Item{{ID: 8001, Kind: "laser", Pos: proto.Vec3{4, 0, 25}}}
	b.Started = time.Now()
	b.observe(o)
	if loot := b.chooseLoot(o, 1, false); loot != nil {
		t.Fatalf("ground bot seeks roof loot: %+v", loot)
	}
}
func TestLiveDimensionsNotScaledTwice(t *testing.T) {
	b, o := fixture()
	o.Self.Cubes = 10
	o.Self.Stats["radius"] = 3.75
	o.Self.Stats["height"] = 13.5
	r, h := b.body(o.Self)
	if r != 3.75 || h != 13.5 {
		t.Fatalf("scaled live dimensions twice: %v %v", r, h)
	}
}
func TestFutureZoneUsesLiveCenter(t *testing.T) {
	b, _ := fixture()
	b.Now = 80
	z := proto.Zone{Center: proto.Vec2{20, 0}, Radius: 200, NextCenter: proto.Vec2{40, 0}, NextRadius: 100, ShrinkStart: 60, ShrinkEnd: 100}
	c, r := b.circle(z, 90)
	if c[0] != 30 || r != 150 {
		t.Fatalf("double interpolated zone: %v %v", c, r)
	}
}
func TestProjectilesAboveBodyDoNotDodge(t *testing.T) {
	b, o := fixture()
	o.Projectiles = []proto.Projectile{{Kind: "gun", Pos: proto.Vec3{-20, 0, 100}, Vel: proto.Vec3{100, 0, 0}}}
	if _, _, ok := b.dodge(o); ok {
		t.Fatal("dodged projectile far above body")
	}
}
func TestGunDoesNotFireFarFromAim(t *testing.T) {
	b, o := fixture()
	o.Mechs = []proto.Mech{enemy(80, 0)}
	o.Self.Aim = proto.Vec2{math.Pi, 0}
	if a := b.Act(o); a.Fire[0] {
		t.Fatal("gun fired before torso can align")
	}
}
func TestNilWeaponSlotsDoNotPanic(t *testing.T) {
	b, o := fixture()
	o.Self.Weapons = []*proto.Weapon{nil, nil}
	o.Mechs = []proto.Mech{enemy(30, 0)}
	a := b.Act(o)
	if math.IsNaN(a.Aim[0]) || math.IsNaN(a.Move[0]) {
		t.Fatal("non-finite action")
	}
}

func BenchmarkDenseAct(bm *testing.B) {
	b, o := fixture()
	for i := 0; i < 15; i++ {
		a := float64(i) * 2 * math.Pi / 15
		m := enemy(math.Cos(a)*80, math.Sin(a)*80)
		m.ID = i + 1
		o.Mechs = append(o.Mechs, m)
	}
	for i := 0; i < 100; i++ {
		a := float64(i)
		o.Projectiles = append(o.Projectiles, proto.Projectile{Kind: "gun", Pos: proto.Vec3{math.Cos(a) * 25, math.Sin(a) * 25, 5}, Vel: proto.Vec3{-math.Cos(a) * 252, -math.Sin(a) * 252, 0}})
	}
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		o.T += 0.1
		o.Tick += 2
		b.Act(o)
	}
}

func TestDenseActualMapBudget(t *testing.T) {
	dir := os.Getenv("MECH_MAP")
	if dir == "" {
		t.Skip("set MECH_MAP for the dense real-map timing check")
	}
	b, o := fixture()
	w, err := world.New(dir, b.Mech)
	if err != nil {
		t.Fatal(err)
	}
	b.World = w
	p := proto.Vec2{-35, -70}
	c := w.Nearest(w.Cell(p))
	if c < 0 {
		t.Fatal("map has no walkable cell")
	}
	p = w.Pos(c)
	o.Self.Pos = at(p, w.Ground(p[0], p[1]))
	o.Self.Cubes = 12
	o.Self.Stats["radius"] = 4
	o.Self.Stats["height"] = 14.4
	o.Self.HP = 600
	o.Self.MaxHP = 1800
	o.T = 100
	for i := 0; i < 15; i++ {
		ang := float64(i) * 2 * math.Pi / 15
		q := movePoint(p, proto.Vec2{math.Cos(ang), math.Sin(ang)}, 65)
		m := enemy(q[0], q[1])
		m.ID = i + 1
		m.Pos[2] = w.Ground(q[0], q[1])
		m.Cubes = 8
		m.Charging = i%3 == 0
		m.Weapons = []string{"gun", "laser"}
		m.Aim[0] = ang + math.Pi
		o.Mechs = append(o.Mechs, m)
	}
	for i := 0; i < 100; i++ {
		ang := float64(i)
		o.Projectiles = append(o.Projectiles, proto.Projectile{Kind: "gun", Pos: proto.Vec3{p[0] + math.Cos(ang)*25, p[1] + math.Sin(ang)*25, o.Self.Pos[2] + 5}, Vel: proto.Vec3{-math.Cos(ang) * 252, -math.Sin(ang) * 252, 0}})
	}
	times := make([]time.Duration, 200)
	late := 0
	for i := range times {
		o.T += 0.1
		o.Tick += 2
		start := time.Now()
		b.Act(o)
		times[i] = time.Since(start)
		if times[i] > 50*time.Millisecond {
			late++
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	t.Logf("dense real map: p99=%s max=%s late=%d/200", times[198], times[199], late)
	if late > 4 {
		t.Fatal("more than2% dense decisions exceeded50ms")
	}
}
