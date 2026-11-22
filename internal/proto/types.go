// Package proto defines the additive mechfight/1 JSON-lines contract.
package proto

import "math"

type Vec2 [2]float64
type Vec3 [3]float64
type Stats map[string]float64

func (s Stats) Get(key string, fallback float64) float64 {
	if v, ok := s[key]; ok {
		return v
	}
	return fallback
}
func (v Vec3) XY() Vec2                { return Vec2{v[0], v[1]} }
func (v Vec2) Distance(w Vec2) float64 { return math.Hypot(v[0]-w[0], v[1]-w[1]) }

type Hello struct {
	Type     string `json:"type"`
	Protocol string `json:"protocol"`
	Slot     int    `json:"slot"`
	Seed     int64  `json:"seed"`
	Map      struct {
		Dir string `json:"dir"`
	} `json:"map"`
	Config map[string]any `json:"config"`
}

func (h Hello) Group(name string) Stats {
	s := Stats{}
	if g, ok := h.Config[name].(map[string]any); ok {
		for k, v := range g {
			if n, ok := v.(float64); ok {
				s[k] = n
			}
		}
	}
	return s
}
func (h Hello) Weapon(name string) Stats {
	s := Stats{}
	if g, ok := h.Config["weapons"].(map[string]any); ok {
		if w, ok := g[name].(map[string]any); ok {
			for k, v := range w {
				if n, ok := v.(float64); ok {
					s[k] = n
				}
			}
		}
	}
	return s
}

type Weapon struct {
	Kind     string  `json:"kind"`
	Cooldown float64 `json:"cooldown"`
	Ammo     int     `json:"ammo"`
	Reload   float64 `json:"reload"`
	Charge   float64 `json:"charge"`
	Stats    Stats   `json:"stats"`
}
type Self struct {
	ID        int       `json:"id"`
	Pos       Vec3      `json:"pos"`
	Vel       Vec3      `json:"vel"`
	Aim       Vec2      `json:"aim"`
	HP        float64   `json:"hp"`
	MaxHP     float64   `json:"max_hp"`
	Energy    float64   `json:"energy"`
	MaxEnergy float64   `json:"max_energy"`
	Grounded  bool      `json:"grounded"`
	Cubes     int       `json:"cubes"`
	InZone    bool      `json:"in_zone"`
	Stats     Stats     `json:"stats"`
	Weapons   []*Weapon `json:"weapons"`
}

func (s Self) Weapon(kind string) (int, *Weapon) {
	for i, w := range s.Weapons {
		if w != nil && w.Kind == kind {
			return i, w
		}
	}
	return -1, nil
}

type Mech struct {
	ID       int      `json:"id"`
	Kind     string   `json:"kind"`
	Pos      Vec3     `json:"pos"`
	Vel      Vec3     `json:"vel"`
	Aim      Vec2     `json:"aim"`
	HP       float64  `json:"hp"`
	MaxHP    float64  `json:"max_hp"`
	Cubes    int      `json:"cubes"`
	Weapons  []string `json:"weapons"`
	Charging bool     `json:"charging"`
}

func (m Mech) Has(kind string) bool {
	for _, w := range m.Weapons {
		if w == kind {
			return true
		}
	}
	return false
}

type Item struct {
	ID   int     `json:"id"`
	Kind string  `json:"kind"`
	Pos  Vec3    `json:"pos"`
	HP   float64 `json:"hp"`
}
type Projectile struct {
	Kind string `json:"kind"`
	Pos  Vec3   `json:"pos"`
	Vel  Vec3   `json:"vel"`
}
type Event struct {
	Type    string   `json:"type"`
	Weapon  string   `json:"weapon"`
	By      *int     `json:"by"`
	Target  int      `json:"target"`
	Bearing *float64 `json:"bearing"`
	Amount  float64  `json:"amount"`
}
type Drop struct {
	ID       int     `json:"id"`
	Pos      Vec3    `json:"pos"`
	Radius   float64 `json:"radius"`
	LandsIn  float64 `json:"lands_in"`
	CaptureS float64 `json:"capture_s"`
	Progress float64 `json:"progress"`
	Holder   *int    `json:"holder"`
}
type Zone struct {
	Center      Vec2    `json:"center"`
	Radius      float64 `json:"radius"`
	NextCenter  Vec2    `json:"next_center"`
	NextRadius  float64 `json:"next_radius"`
	ShrinkStart float64 `json:"shrink_start"`
	ShrinkEnd   float64 `json:"shrink_end"`
	DPS         float64 `json:"dps"`
}
type Choice struct {
	ID string `json:"id"`
}
type Offer struct {
	Choices []Choice `json:"choices"`
}
type Observation struct {
	Type        string       `json:"type"`
	Tick        int          `json:"tick"`
	T           float64      `json:"t"`
	Alive       int          `json:"alive"`
	Self        Self         `json:"self"`
	Mechs       []Mech       `json:"mechs"`
	Projectiles []Projectile `json:"projectiles"`
	Crates      []Item       `json:"crates"`
	Cubes       []Item       `json:"cubes"`
	Pickups     []Item       `json:"pickups"`
	Events      []Event      `json:"events"`
	Drops       []Drop       `json:"drops"`
	Zone        Zone         `json:"zone"`
	Offer       *Offer       `json:"offer"`
}
type Action struct {
	Tick   int     `json:"tick"`
	Move   Vec2    `json:"move"`
	Aim    Vec2    `json:"aim"`
	Fire   [2]bool `json:"fire"`
	Boost  bool    `json:"boost"`
	Jump   bool    `json:"jump,omitempty"`
	Reload bool    `json:"reload,omitempty"`
	Pick   *int    `json:"pick,omitempty"`
	Take   *int    `json:"take,omitempty"`
	Drop   *int    `json:"drop,omitempty"`
}
