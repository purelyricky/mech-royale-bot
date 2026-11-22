// Package world builds the map's terrain/prism columns and bounded navigation.
// Trees are deliberately absent: they obstruct vision, but never weapon shots.
package world

import (
	"apexbot/internal/proto"
	"container/heap"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

type World struct {
	Ready                         bool
	MinX, MinY, Step              float64
	NX, NY                        int
	Grounds, Tops, Low, High      []float32
	NavX, NavY                    int
	NavStep                       float64
	Blocked                       []bool
	Surface                       []float64
	Component                     []int
	MainComponent                 int
	NearMain                      []int
	Radius, Height, StepUp, Slope float64
}
type mapData struct {
	Terrain struct {
		MinX, MinY, Step, Scale float64
		NX, NY                  int
	} `json:"terrain"`
	Prisms []struct {
		Base, Top float64
		Ring      []float64
		Holes     [][]float64
	} `json:"prisms"`
}

func New(dir string, cfg proto.Stats) (*World, error) {
	w := &World{Step: 1, NavStep: 3, Radius: cfg.Get("radius", 2.5), Height: cfg.Get("height", 9), StepUp: cfg.Get("step_up", 1.2), Slope: math.Tan(cfg.Get("max_slope_deg", 50) * math.Pi / 180)}
	data, err := os.ReadFile(filepath.Join(dir, "v2", "world.json"))
	if err != nil {
		return w, err
	}
	var m mapData
	if err = json.Unmarshal(data, &m); err != nil {
		return w, err
	}
	w.MinX, w.MinY, w.Step, w.NX, w.NY = m.Terrain.MinX, m.Terrain.MinY, m.Terrain.Step, m.Terrain.NX, m.Terrain.NY
	if w.Step <= 0 || w.NX < 2 || w.NY < 2 || w.NX*w.NY > 4_000_000 {
		return w, fmt.Errorf("invalid terrain dimensions")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "v2", "terrain.i16"))
	if err != nil {
		return w, err
	}
	n := w.NX * w.NY
	if len(raw) < n*2 {
		return w, fmt.Errorf("truncated terrain")
	}
	scale := m.Terrain.Scale
	if scale == 0 {
		scale = 0.01
	}
	w.Grounds = make([]float32, n)
	w.Tops = make([]float32, n)
	w.Low = make([]float32, n)
	w.High = make([]float32, n)
	for k := range w.Grounds {
		z := float32(float64(int16(binary.LittleEndian.Uint16(raw[k*2:]))) * scale)
		w.Grounds[k] = z
		w.Tops[k] = z
		w.Low[k] = float32(math.Inf(1))
		w.High[k] = float32(math.Inf(-1))
	}
	for _, p := range m.Prisms {
		if len(p.Ring) < 6 || p.Top <= p.Base {
			continue
		}
		minx, miny, maxx, maxy := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for i := 0; i+1 < len(p.Ring); i += 2 {
			minx = math.Min(minx, p.Ring[i])
			maxx = math.Max(maxx, p.Ring[i])
			miny = math.Min(miny, p.Ring[i+1])
			maxy = math.Max(maxy, p.Ring[i+1])
		}
		for j := max(0, int(math.Floor((miny-w.MinY)/w.Step))); j <= min(w.NY-1, int(math.Ceil((maxy-w.MinY)/w.Step))); j++ {
			for i := max(0, int(math.Floor((minx-w.MinX)/w.Step))); i <= min(w.NX-1, int(math.Ceil((maxx-w.MinX)/w.Step))); i++ {
				x, y := w.MinX+float64(i)*w.Step, w.MinY+float64(j)*w.Step
				if !inside(p.Ring, x, y) {
					continue
				}
				hole := false
				for _, h := range p.Holes {
					if inside(h, x, y) {
						hole = true
						break
					}
				}
				if hole {
					continue
				}
				k := j*w.NX + i
				if p.Base <= float64(w.Grounds[k])+1.5 {
					w.Tops[k] = max(w.Tops[k], float32(p.Top))
				} else {
					w.Low[k] = min(w.Low[k], float32(p.Base))
					w.High[k] = max(w.High[k], float32(p.Top))
				}
			}
		}
	}
	w.Ready = true
	w.buildNav()
	return w, nil
}
func Flat() *World { return &World{Radius: 2.5, Height: 9, StepUp: 1.2, Slope: 1.19, NavStep: 3} }
func inside(r []float64, x, y float64) bool {
	hit := false
	n := len(r) / 2
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi, xj, yj := r[2*i], r[2*i+1], r[2*j], r[2*j+1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			hit = !hit
		}
	}
	return hit
}
func (w *World) index(x, y float64) int {
	i := int(math.Round((x - w.MinX) / w.Step))
	j := int(math.Round((y - w.MinY) / w.Step))
	if i < 0 || j < 0 || i >= w.NX || j >= w.NY {
		return -1
	}
	return j*w.NX + i
}
func (w *World) Ground(x, y float64) float64 {
	if !w.Ready {
		return 0
	}
	fx, fy := (x-w.MinX)/w.Step, (y-w.MinY)/w.Step
	i, j := int(math.Floor(fx)), int(math.Floor(fy))
	i = max(0, min(w.NX-2, i))
	j = max(0, min(w.NY-2, j))
	a, b := math.Max(0, math.Min(1, fx-float64(i))), math.Max(0, math.Min(1, fy-float64(j)))
	k := j*w.NX + i
	return (1-b)*((1-a)*float64(w.Grounds[k])+a*float64(w.Grounds[k+1])) + b*((1-a)*float64(w.Grounds[k+w.NX])+a*float64(w.Grounds[k+w.NX+1]))
}
func (w *World) WalkAt(x, y float64) float64 {
	if !w.Ready {
		return 0
	}
	k := w.index(x, y)
	if k < 0 {
		return 0
	}
	return math.Max(w.Ground(x, y), float64(w.Tops[k]))
}
func (w *World) solid(x, y, z float64) bool {
	k := w.index(x, y)
	if k < 0 {
		return false
	}
	return z < w.Ground(x, y) || z < float64(w.Tops[k]) || (z >= float64(w.Low[k]) && z <= float64(w.High[k]))
}
func (w *World) Clear(a, b proto.Vec3) bool {
	if !w.Ready {
		return true
	}
	dx, dy, dz := b[0]-a[0], b[1]-a[1], b[2]-a[2]
	n := max(2, int(math.Ceil(math.Hypot(dx, dy)/0.8)))
	for i := 1; i < n; i++ {
		f := float64(i) / float64(n)
		if w.solid(a[0]+dx*f, a[1]+dy*f, a[2]+dz*f) {
			return false
		}
	}
	return true
}
func (w *World) body(x, y, z, r, h float64) bool {
	if !w.Ready {
		return true
	}
	for i := -1; i < 8; i++ {
		px, py := x, y
		if i >= 0 {
			a := float64(i) * math.Pi / 4
			px += math.Cos(a) * r * 0.95
			py += math.Sin(a) * r * 0.95
		}
		k := w.index(px, py)
		if k < 0 {
			return false
		}
		if float64(w.Tops[k]) > z+w.StepUp+0.05 || float64(w.Low[k]) < z+h+0.15 && float64(w.High[k]) > z+w.StepUp {
			return false
		}
	}
	return true
}
func (w *World) WalkClear(a, b proto.Vec2, z, r, h float64) bool {
	if !w.Ready {
		return true
	}
	d := a.Distance(b)
	n := max(1, int(math.Ceil(d/1.0)))
	for i := 1; i <= n; i++ {
		f := float64(i) / float64(n)
		x, y := a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f
		g := w.WalkAt(x, y)
		if g-z > w.StepUp+0.05 && g-z > w.Slope*d/float64(n)+0.05 {
			return false
		}
		z = math.Max(g, z-math.Max(1, d/float64(n)*w.Slope))
		if !w.body(x, y, z, r, h) {
			return false
		}
	}
	return true
}
func (w *World) buildNav() {
	w.NavX = int(float64(w.NX-1)*w.Step/w.NavStep) + 1
	w.NavY = int(float64(w.NY-1)*w.Step/w.NavStep) + 1
	n := w.NavX * w.NavY
	w.Blocked = make([]bool, n)
	w.Surface = make([]float64, n)
	w.Component = make([]int, n)
	for c := 0; c < n; c++ {
		p := w.Pos(c)
		z := w.Ground(p[0], p[1])
		w.Surface[c] = z
		w.Blocked[c] = !w.body(p[0], p[1], z, w.Radius, w.Height)
	}
	best := 0
	component := 0
	for s := 0; s < n; s++ {
		if w.Blocked[s] || w.Component[s] != 0 {
			continue
		}
		component++
		q := []int{s}
		w.Component[s] = component
		for at := 0; at < len(q); at++ {
			c := q[at]
			for _, nb := range w.neighbors(c, false) {
				if w.Component[nb] == 0 && w.edge(c, nb) {
					w.Component[nb] = component
					q = append(q, nb)
				}
			}
		}
		if len(q) > best {
			best = len(q)
			w.MainComponent = component
		}
	}
	// Jump-jet escape planning may cross walls; ground navigation never does.
	w.NearMain = make([]int, n)
	q := make([]int, 0, n)
	for c := 0; c < n; c++ {
		w.NearMain[c] = -1
		if w.Component[c] == w.MainComponent && w.MainComponent != 0 {
			w.NearMain[c] = c
			q = append(q, c)
		}
	}
	for at := 0; at < len(q); at++ {
		c := q[at]
		x, y := c%w.NavX, c/w.NavX
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			i, j := x+d[0], y+d[1]
			if i >= 0 && j >= 0 && i < w.NavX && j < w.NavY {
				nb := j*w.NavX + i
				if w.NearMain[nb] < 0 {
					w.NearMain[nb] = w.NearMain[c]
					q = append(q, nb)
				}
			}
		}
	}
}

func (w *World) Escape(p proto.Vec2) (proto.Vec2, proto.Vec2, bool) {
	if !w.Ready {
		return p, p, false
	}
	s := w.Nearest(w.Cell(p))
	if s < 0 || w.Component[s] == w.MainComponent {
		return p, p, false
	}
	component := w.Component[s]
	score := math.Inf(1)
	launch, land := p, p
	for c, comp := range w.Component {
		if comp != component {
			continue
		}
		m := w.NearMain[c]
		if m < 0 {
			continue
		}
		a, z := w.Pos(c), w.Pos(m)
		d := a.Distance(z)
		if d > 80 {
			continue
		}
		cost := p.Distance(a) + d*4
		if cost >= score {
			continue
		}
		need := w.TopAlong(a, z)
		if need-w.Surface[c] > 55 {
			continue
		}
		score, launch, land = cost, a, z
	}
	return launch, land, !math.IsInf(score, 1)
}
func (w *World) TopAlong(a, b proto.Vec2) float64 {
	if !w.Ready {
		return 0
	}
	n := max(1, int(math.Ceil(a.Distance(b))))
	need := math.Inf(-1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		k := w.index(a[0]+(b[0]-a[0])*f, a[1]+(b[1]-a[1])*f)
		if k >= 0 {
			need = math.Max(need, math.Max(float64(w.Tops[k]), float64(w.High[k])))
		}
	}
	return need
}
func (w *World) Cell(p proto.Vec2) int {
	if !w.Ready {
		return -1
	}
	i := max(0, min(w.NavX-1, int(math.Round((p[0]-w.MinX)/w.NavStep))))
	j := max(0, min(w.NavY-1, int(math.Round((p[1]-w.MinY)/w.NavStep))))
	return j*w.NavX + i
}
func (w *World) Pos(c int) proto.Vec2 {
	return proto.Vec2{w.MinX + float64(c%w.NavX)*w.NavStep, w.MinY + float64(c/w.NavX)*w.NavStep}
}
func (w *World) Nearest(c int) int {
	if !w.Ready {
		return -1
	}
	if c >= 0 && c < len(w.Blocked) && !w.Blocked[c] {
		return c
	}
	if c < 0 {
		return -1
	}
	x, y := c%w.NavX, c/w.NavX
	for r := 1; r <= 8; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) != r && abs(dy) != r {
					continue
				}
				i, j := x+dx, y+dy
				if i >= 0 && j >= 0 && i < w.NavX && j < w.NavY {
					nb := j*w.NavX + i
					if !w.Blocked[nb] {
						return nb
					}
				}
			}
		}
	}
	return -1
}
func (w *World) Reachable(p proto.Vec2) bool {
	if !w.Ready {
		return true
	}
	c := w.Nearest(w.Cell(p))
	return c >= 0 && w.Component[c] == w.MainComponent
}
func (w *World) neighbors(c int, diag bool) []int {
	out := make([]int, 0, 8)
	x, y := c%w.NavX, c/w.NavX
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 || !diag && dx != 0 && dy != 0 {
				continue
			}
			i, j := x+dx, y+dy
			if i < 0 || j < 0 || i >= w.NavX || j >= w.NavY {
				continue
			}
			nb := j*w.NavX + i
			if w.Blocked[nb] {
				continue
			}
			if dx != 0 && dy != 0 && (w.Blocked[y*w.NavX+i] || w.Blocked[j*w.NavX+x]) {
				continue
			}
			out = append(out, nb)
		}
	}
	return out
}
func (w *World) edge(c, nb int) bool {
	p, q := w.Pos(c), w.Pos(nb)
	rise := w.Surface[nb] - w.Surface[c]
	return math.Abs(rise) <= math.Max(w.StepUp, w.Slope*p.Distance(q)) && w.WalkClear(p, q, w.Surface[c], w.Radius, w.Height)
}

type entry struct {
	c    int
	f, g float64
}
type queue []entry

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].f == q[j].f {
		return q[i].c < q[j].c
	}
	return q[i].f < q[j].f
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(v any)   { *q = append(*q, v.(entry)) }
func (q *queue) Pop() any     { old := *q; n := len(old); v := old[n-1]; *q = old[:n-1]; return v }
func (w *World) Path(a, b proto.Vec2, budget time.Duration) []proto.Vec2 {
	if !w.Ready {
		return []proto.Vec2{b}
	}
	s, g := w.Nearest(w.Cell(a)), w.Nearest(w.Cell(b))
	if s < 0 || g < 0 {
		return nil
	}
	n := len(w.Blocked)
	dist := make([]float64, n)
	parent := make([]int, n)
	for i := range dist {
		dist[i] = math.Inf(1)
		parent[i] = -1
	}
	dist[s] = 0
	goal := w.Pos(g)
	h := func(c int) float64 { return w.Pos(c).Distance(goal) }
	q := &queue{{s, h(s), 0}}
	heap.Init(q)
	best, bh := s, h(s)
	start := time.Now()
	for expanded := 0; q.Len() > 0 && expanded < 4000; expanded++ {
		if expanded&63 == 63 && time.Since(start) > budget {
			break
		}
		it := heap.Pop(q).(entry)
		c := it.c
		if it.g != dist[c] {
			continue
		}
		if c == g {
			best = c
			break
		}
		if hc := h(c); hc < bh {
			best, bh = c, hc
		}
		for _, nb := range w.neighbors(c, true) {
			if !w.edge(c, nb) {
				continue
			}
			nd := dist[c] + w.Pos(c).Distance(w.Pos(nb))
			if nd < dist[nb] {
				dist[nb] = nd
				parent[nb] = c
				heap.Push(q, entry{nb, nd + h(nb)*1.10, nd})
			}
		}
	}
	var path []proto.Vec2
	for c := best; c >= 0; c = parent[c] {
		path = append(path, w.Pos(c))
		if c == s {
			break
		}
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// Cover searches connected, physically reachable cells, never a point through a wall.
func (w *World) Cover(a proto.Vec2, score func(proto.Vec2) float64, budget time.Duration) (proto.Vec2, bool) {
	if !w.Ready {
		return a, false
	}
	s := w.Nearest(w.Cell(a))
	if s < 0 {
		return a, false
	}
	seen := map[int]bool{s: true}
	q := []int{s}
	best, bestScore := a, score(a)
	found := bestScore < 100
	start := time.Now()
	for at := 0; at < len(q) && at < 500; at++ {
		if at&31 == 31 && time.Since(start) > budget {
			break
		}
		c := q[at]
		p := w.Pos(c)
		if p.Distance(a) > 90 {
			continue
		}
		if at%2 == 0 {
			v := score(p) + p.Distance(a)*0.6
			if v < bestScore {
				best, bestScore, found = p, v, true
			}
		}
		for _, nb := range w.neighbors(c, false) {
			if !seen[nb] && w.edge(c, nb) {
				seen[nb] = true
				q = append(q, nb)
			}
		}
	}
	return best, found
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
