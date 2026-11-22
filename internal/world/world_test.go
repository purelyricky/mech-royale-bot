package world

import (
	"apexbot/internal/proto"
	"math"
	"testing"
	"time"
)

func testWorld() *World {
	n := 61 * 61
	w := &World{Ready: true, NX: 61, NY: 61, Step: 1, NavStep: 3, Radius: 2.5, Height: 9, StepUp: 1.2, Slope: 1.19, Grounds: make([]float32, n), Tops: make([]float32, n), Low: make([]float32, n), High: make([]float32, n)}
	for i := range w.Low {
		w.Low[i] = float32(math.Inf(1))
		w.High[i] = float32(math.Inf(-1))
	}
	return w
}
func TestBridgeClearanceAndShots(t *testing.T) {
	w := testWorld()
	for y := 0; y < 61; y++ {
		k := y*61 + 30
		w.Low[k] = 12
		w.High[k] = 15
	}
	if !w.Clear(proto.Vec3{20, 30, 7}, proto.Vec3{40, 30, 7}) {
		t.Fatal("shot below bridge blocked")
	}
	if w.Clear(proto.Vec3{20, 30, 13}, proto.Vec3{40, 30, 13}) {
		t.Fatal("shot through bridge slab allowed")
	}
	if !w.WalkClear(proto.Vec2{20, 30}, proto.Vec2{40, 30}, 0, 2.5, 9) {
		t.Fatal("base mech should fit under bridge")
	}
	if w.WalkClear(proto.Vec2{20, 30}, proto.Vec2{40, 30}, 0, 3.75, 13.5) {
		t.Fatal("grown mech should not fit under bridge")
	}
}
func TestPathRoutesAroundSolidWall(t *testing.T) {
	w := testWorld()
	for y := 0; y < 42; y++ {
		w.Tops[y*61+30] = 20
	}
	w.buildNav()
	a, z := proto.Vec2{15, 15}, proto.Vec2{45, 15}
	if w.WalkClear(a, z, 0, 2.5, 9) {
		t.Fatal("direct walk crosses wall")
	}
	path := w.Path(a, z, 100*time.Millisecond)
	if len(path) < 3 || path[len(path)-1].Distance(z) > 5 {
		t.Fatalf("no complete detour: %v", path)
	}
	for i := 1; i < len(path); i++ {
		if !w.WalkClear(path[i-1], path[i], 0, 2.5, 9) {
			t.Fatal("path contains blocked edge")
		}
	}
}
func TestCourtyardEscapeFindsMainGround(t *testing.T) {
	w := testWorld()
	for y := 15; y <= 39; y++ {
		for x := 15; x <= 39; x++ {
			if x == 15 || x == 39 || y == 15 || y == 39 {
				w.Tops[y*61+x] = 18
			}
		}
	}
	w.buildNav()
	p := proto.Vec2{27, 27}
	if w.Reachable(p) {
		t.Fatal("courtyard unexpectedly connected")
	}
	launch, land, ok := w.Escape(p)
	if !ok || !w.Reachable(land) || launch.Distance(land) > 40 {
		t.Fatalf("bad escape plan: %v %v %v", launch, land, ok)
	}
}
func TestBilinearTerrain(t *testing.T) {
	w := testWorld()
	w.Grounds[10*61+10] = 0
	w.Grounds[10*61+11] = 2
	w.Grounds[11*61+10] = 4
	w.Grounds[11*61+11] = 6
	if g := w.Ground(10.5, 10.5); g != 3 {
		t.Fatalf("expected interpolated3 got%v", g)
	}
}
func TestBoundsRejectBody(t *testing.T) {
	w := testWorld()
	if w.body(0, 0, 0, 2.5, 9) {
		t.Fatal("body extends outside map")
	}
}
