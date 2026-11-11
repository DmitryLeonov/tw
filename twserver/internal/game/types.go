package game

import "time"

type Config struct {
	MaxPlayers  int
	MaxTurns    int
	TurnTimeout time.Duration
	Width       int
	Height      int
}

type Player struct {
	ID         string `json:"player_id"`
	Name       string `json:"name"`
	Token      string `json:"-"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Energy     int    `json:"energy"`
	Score      int    `json:"score"`
	Connected  bool   `json:"connected"`
	Submitted  bool   `json:"submitted"`
	Eliminated bool   `json:"eliminated"`
}

type Cell struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Owner  string `json:"owner,omitempty"`
	Health int    `json:"health"`
	Type   string `json:"type"`
}

type Observation struct {
	MatchID          string         `json:"match_id"`
	PlayerID         string         `json:"player_id"`
	Turn             int            `json:"turn"`
	GameOver         bool           `json:"game_over"`
	Width            int            `json:"width"`
	Height           int            `json:"height"`
	Energy           int            `json:"energy"`
	Score            int            `json:"score"`
	Cells            []Cell         `json:"cells"`
	Players          []PublicPlayer `json:"players"`
	AvailableActions []string       `json:"available_actions"`
	ExpandTargets    []Point        `json:"expand_targets,omitempty"`
	AttackTargets    []Point        `json:"attack_targets,omitempty"`
	FortifyTargets   []Point        `json:"fortify_targets,omitempty"`
}

type PublicPlayer struct {
	ID         string `json:"player_id"`
	Name       string `json:"name"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Score      int    `json:"score"`
	Energy     int    `json:"energy"`
	Territory  int    `json:"territory"`
	Connected  bool   `json:"connected"`
	Eliminated bool   `json:"eliminated"`
}

type Action struct {
	Type   string `json:"type"`
	Target *Point `json:"target,omitempty"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type ActionMessage struct {
	Type    string   `json:"type"`
	Turn    int      `json:"turn"`
	Actions []Action `json:"actions"`
}

type Event struct {
	Type        string       `json:"type"`
	Turn        int          `json:"turn,omitempty"`
	Message     string       `json:"message,omitempty"`
	Observation *Observation `json:"observation,omitempty"`
	Results     []Result     `json:"results,omitempty"`
}

type Result struct {
	Turn     int    `json:"turn"`
	PlayerID string `json:"player_id"`
	Action   Action `json:"action"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}
