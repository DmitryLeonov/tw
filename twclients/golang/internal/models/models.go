package models

// Cell represents a single cell on the game map.
type Cell struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Owner  string `json:"owner,omitempty"`
	Health int    `json:"health"`
	Type   string `json:"type"`
}

// PublicPlayer represents a player as seen in the observation.
type PublicPlayer struct {
	PlayerID   string `json:"player_id"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Score      int    `json:"score"`
	Connected  bool   `json:"connected"`
	Eliminated bool   `json:"eliminated"`
}

// Observation is the full game state snapshot returned by the server.
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
}

// Action represents a single game action to submit.
type Action struct {
	Type   string `json:"type"`
	Target *Point `json:"target,omitempty"`
}

// Point is a coordinate on the game map.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ActionMessage is the JSON payload sent over WebSocket each turn.
type ActionMessage struct {
	Type    string   `json:"type"`
	Turn    int      `json:"turn"`
	Actions []Action `json:"actions"`
}

// JoinResponse is returned by POST /api/v1/join.
type JoinResponse struct {
	PlayerID     string `json:"player_id"`
	Token        string `json:"token"`
	WebsocketURL string `json:"websocket_url"`
}

// Event is a server-to-client event received over WebSocket.
type Event struct {
	Type    string      `json:"type"`
	Turn    int         `json:"turn,omitempty"`
	Message string      `json:"message,omitempty"`
	Results []Result    `json:"results,omitempty"`
}

// Result is a single action result in a turn.
type Result struct {
	PlayerID string `json:"player_id"`
	Type     string `json:"action_type"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}
