package game

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Engine manages the authoritative game state. All turn resolution happens
// inside the engine; the Round Manager only observes state changes and
// broadcasts events to connected clients.
type Engine struct {
	mu            sync.RWMutex
	cfg           Config
	matchID       string
	turn          int
	gameOver      bool
	cells         map[Point]*Cell
	players       map[string]*Player
	tokenToPlayer map[string]string
	pending       map[string]Action
	results       []Result
	roundStarted  time.Time

	// turnChanged is recreated after every resolution. Callers must use
	// TurnSignal() to get a fresh channel each time.
	turnChanged   chan struct{}
	turnChangedMu sync.Mutex // protects turnChanged creation/closure
}

func NewEngine(cfg Config) *Engine {
	if cfg.MaxPlayers < 2 {
		cfg.MaxPlayers = 20
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 200
	}
	if cfg.TurnTimeout <= 0 {
		cfg.TurnTimeout = 15 * time.Second
	}
	if cfg.Width < 10 {
		cfg.Width = 40
	}
	if cfg.Height < 10 {
		cfg.Height = 40
	}
	id := randomHex(8)
	e := &Engine{
		cfg: cfg, matchID: id, cells: make(map[Point]*Cell),
		players: make(map[string]*Player), tokenToPlayer: make(map[string]string),
		pending: make(map[string]Action), roundStarted: time.Now(),
		turnChanged: make(chan struct{}),
	}
	e.initMap()
	return e
}

func (e *Engine) initMap() {
	for y := 0; y < e.cfg.Height; y++ {
		for x := 0; x < e.cfg.Width; x++ {
			kind := "normal"
			if (x*17+y*31)%37 == 0 {
				kind = "resource"
			}
			e.cells[Point{x, y}] = &Cell{X: x, Y: y, Health: 3, Type: kind}
		}
	}
}

func (e *Engine) Join(name string) (*Player, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.players) >= e.cfg.MaxPlayers {
		return nil, errors.New("match is full")
	}
	if e.turn > 0 || len(e.pending) > 0 {
		return nil, errors.New("registration is closed after the first turn starts")
	}
	if name == "" {
		name = fmt.Sprintf("Player-%d", len(e.players)+1)
	}
	id := fmt.Sprintf("p%02d", len(e.players)+1)
	token := randomHex(24)
	n := len(e.players)
	if n == 0 {
		e.roundStarted = time.Now()
	}
	positions := spawnPositions(e.cfg.Width, e.cfg.Height, e.cfg.MaxPlayers)
	pos := positions[n]
	p := &Player{ID: id, Name: name, Token: token, X: pos.X, Y: pos.Y, Energy: 5, Connected: false}
	e.players[id] = p
	e.tokenToPlayer[token] = id
	cell := e.cells[pos]
	cell.Owner, cell.Health, cell.Type = id, 5, "base"
	return clonePlayer(p), nil
}

func spawnPositions(w, h, count int) []Point {
	out := make([]Point, 0, count)
	cx, cy := float64(w-1)/2, float64(h-1)/2
	rx, ry := float64(w)*0.42, float64(h)*0.42
	for i := 0; i < count; i++ {
		a := 2*math.Pi*float64(i)/float64(count) - math.Pi/2
		out = append(out, Point{int(math.Round(cx + rx*math.Cos(a))), int(math.Round(cy + ry*math.Sin(a)))})
	}
	return out
}

func (e *Engine) Authenticate(token string) (*Player, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	id, ok := e.tokenToPlayer[token]
	if !ok {
		return nil, false
	}
	p, ok := e.players[id]
	if !ok {
		return nil, false
	}
	return clonePlayer(p), true
}

func (e *Engine) SetConnected(playerID string, connected bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p := e.players[playerID]; p != nil {
		p.Connected = connected
	}
}

func (e *Engine) Observation(playerID string) (*Observation, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	p := e.players[playerID]
	if p == nil {
		return nil, errors.New("unknown player")
	}
	o := &Observation{
		MatchID: e.matchID, PlayerID: playerID, Turn: e.turn, GameOver: e.gameOver,
		Width: e.cfg.Width, Height: e.cfg.Height, Energy: p.Energy, Score: p.Score,
		AvailableActions: []string{"expand", "attack", "fortify", "pass"},
		Cells:            make([]Cell, 0, len(e.cells)), Players: make([]PublicPlayer, 0, len(e.players)),
		ExpandTargets:    e.expandTargets(playerID),
		AttackTargets:    e.attackTargets(playerID),
		FortifyTargets:   e.fortifyTargets(playerID),
	}
	for _, c := range e.cells {
		o.Cells = append(o.Cells, *c)
	}
	sort.Slice(o.Cells, func(i, j int) bool {
		if o.Cells[i].Y == o.Cells[j].Y {
			return o.Cells[i].X < o.Cells[j].X
		}
		return o.Cells[i].Y < o.Cells[j].Y
	})
	for _, other := range e.players {
		o.Players = append(o.Players, PublicPlayer{ID: other.ID, X: other.X, Y: other.Y, Score: other.Score, Connected: other.Connected, Eliminated: other.Eliminated})
	}
	sort.Slice(o.Players, func(i, j int) bool { return o.Players[i].ID < o.Players[j].ID })
	return o, nil
}

// SubmitResult bundles the action submission result with the turn number so
// the Round Manager can atomically queue ACTION_ACCEPTED alongside the
// submission. Returns nil when the action is invalid (no ACCEPTED needed).
type SubmitResult struct {
	Turn        int
	Action      Action
	Accepted    bool
	RejectError error
}

func (e *Engine) Submit(playerID string, msg ActionMessage) *SubmitResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gameOver {
		return &SubmitResult{Turn: e.turn, RejectError: errors.New("game is over")}
	}
	if _, ok := e.players[playerID]; !ok {
		return &SubmitResult{Turn: e.turn, RejectError: errors.New("unknown player")}
	}
	if msg.Type != "ACTION" {
		return &SubmitResult{Turn: e.turn, RejectError: errors.New("message type must be ACTION")}
	}
	if msg.Turn != e.turn {
		return &SubmitResult{Turn: e.turn, RejectError: fmt.Errorf("stale turn: current turn is %d", e.turn)}
	}
	if _, exists := e.pending[playerID]; exists {
		return &SubmitResult{Turn: e.turn, RejectError: errors.New("action already submitted for this turn")}
	}
	if len(msg.Actions) != 1 {
		return &SubmitResult{Turn: e.turn, RejectError: errors.New("exactly one action is required per turn")}
	}

	action := msg.Actions[0]
	e.pending[playerID] = action

	// If all players have submitted, resolve immediately.
	if len(e.pending) == len(e.players) {
		e.resolveTurnLocked()
	}

	return &SubmitResult{
		Turn:     e.turn,
		Action:   action,
		Accepted: true,
	}
}

// ResolveExpiredTurn resolves the current turn if the timeout has elapsed.
// Returns true if a resolution actually happened (prevents double-resolution).
func (e *Engine) ResolveExpiredTurn() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gameOver || len(e.players) == 0 {
		return false
	}
	if time.Since(e.roundStarted) < e.cfg.TurnTimeout {
		return false
	}
	// Only resolve if there are pending actions (i.e. the turn hasn't been
	// resolved already by all-submitted path).
	if len(e.pending) == 0 {
		return false
	}
	e.resolveTurnLocked()
	return true
}

func (e *Engine) resolveTurnLocked() {
	ids := make([]string, 0, len(e.players))
	for id := range e.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// Resolve in rotating player order to avoid a permanent first-player advantage.
	if len(ids) > 0 {
		offset := e.turn % len(ids)
		ids = append(ids[offset:], ids[:offset]...)
	}
	for _, id := range ids {
		p := e.players[id]
		if p.Eliminated {
			continue
		}
		a, ok := e.pending[id]
		if !ok {
			a = Action{Type: "pass"}
		}
		r := e.applyActionLocked(p, a)
		e.results = append(e.results, r)
	}
	for _, p := range e.players {
		if !p.Eliminated {
			income := 1
			for _, c := range e.cells {
				if c.Owner == p.ID && c.Type == "resource" {
					income++
				}
			}
			p.Energy += income
			if p.Energy > 20 {
				p.Energy = 20
			}
		}
		p.Submitted = false
	}
	e.turn++
	e.pending = make(map[string]Action)
	e.roundStarted = time.Now()
	if e.turn >= e.cfg.MaxTurns {
		e.gameOver = true
	}

	// Signal turn completion. Use the mutex to prevent races with
	// TurnSignal() callers who may be reading the channel.
	e.turnChangedMu.Lock()
	close(e.turnChanged)
	e.turnChanged = make(chan struct{})
	e.turnChangedMu.Unlock()
}

func (e *Engine) applyActionLocked(p *Player, a Action) Result {
	r := Result{Turn: e.turn, PlayerID: p.ID, Action: a, Accepted: false}
	if a.Type == "pass" {
		r.Accepted = true
		return r
	}
	if a.Target == nil {
		r.Reason = "target is required"
		return r
	}
	pt := *a.Target
	c := e.cells[pt]
	if c == nil {
		r.Reason = "target is outside the map"
		return r
	}
	switch a.Type {
	case "expand":
		if !e.adjacentToOwnedLocked(p.ID, pt) {
			r.Reason = "target must be adjacent to your territory"
			return r
		}
		if c.Owner != "" {
			r.Reason = "target is not neutral"
			return r
		}
		if p.Energy < 1 {
			r.Reason = "not enough energy"
			return r
		}
		p.Energy--
		c.Owner = p.ID
		c.Health = 3
		p.Score++
		r.Accepted = true
	case "attack":
		if !e.adjacentToOwnedLocked(p.ID, pt) {
			r.Reason = "target must be adjacent to your territory"
			return r
		}
		if c.Owner == "" || c.Owner == p.ID {
			r.Reason = "target must belong to an enemy"
			return r
		}
		if p.Energy < 2 {
			r.Reason = "not enough energy"
			return r
		}
		p.Energy -= 2
		c.Health -= 2
		if c.Health <= 0 {
			if old := e.players[c.Owner]; old != nil {
				old.Score -= 2
				if old.Score < 0 {
					old.Score = 0
				}
			}
			c.Owner = p.ID
			c.Health = 3
			p.Score += 3
		}
		r.Accepted = true
	case "fortify":
		if c.Owner != p.ID {
			r.Reason = "you can only fortify your own cell"
			return r
		}
		if p.Energy < 2 {
			r.Reason = "not enough energy"
			return r
		}
		p.Energy -= 2
		c.Health += 2
		if c.Health > 7 {
			c.Health = 7
		}
		r.Accepted = true
	default:
		r.Reason = "unknown action"
	}
	return r
}

func (e *Engine) adjacentToOwnedLocked(playerID string, target Point) bool {
	neighbors := []Point{
		{target.X - 1, target.Y}, {target.X + 1, target.Y},
		{target.X, target.Y - 1}, {target.X, target.Y + 1},
	}
	for _, pt := range neighbors {
		// Wrap around edges (toroidal map).
		pt.X = (pt.X + e.cfg.Width) % e.cfg.Width
		pt.Y = (pt.Y + e.cfg.Height) % e.cfg.Height
		if c := e.cells[pt]; c != nil && c.Owner == playerID {
			return true
		}
	}
	return false
}

// ExpandTargets returns neutral cells adjacent to the player's territory.
func (e *Engine) ExpandTargets(playerID string) []Point {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.expandTargets(playerID)
}

func (e *Engine) expandTargets(playerID string) []Point {
	var targets []Point
	for _, c := range e.cells {
		if c.Owner == "" && e.adjacentToOwnedLocked(playerID, Point{c.X, c.Y}) {
			targets = append(targets, Point{c.X, c.Y})
		}
	}
	return targets
}

// AttackTargets returns enemy cells adjacent to the player's territory.
func (e *Engine) AttackTargets(playerID string) []Point {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.attackTargets(playerID)
}

func (e *Engine) attackTargets(playerID string) []Point {
	var targets []Point
	for _, c := range e.cells {
		if c.Owner != "" && c.Owner != playerID && e.adjacentToOwnedLocked(playerID, Point{c.X, c.Y}) {
			targets = append(targets, Point{c.X, c.Y})
		}
	}
	return targets
}

// FortifyTargets returns the player's own cells.
func (e *Engine) FortifyTargets(playerID string) []Point {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.fortifyTargets(playerID)
}

func (e *Engine) fortifyTargets(playerID string) []Point {
	var targets []Point
	for _, c := range e.cells {
		if c.Owner == playerID {
			targets = append(targets, Point{c.X, c.Y})
		}
	}
	return targets
}

func (e *Engine) Leaderboard() []PublicPlayer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Count territory per player.
	territory := make(map[string]int, len(e.players))
	for _, c := range e.cells {
		if c.Owner != "" {
			territory[c.Owner]++
		}
	}
	out := make([]PublicPlayer, 0, len(e.players))
	for _, p := range e.players {
		out = append(out, PublicPlayer{
			ID: p.ID, Name: p.Name, X: p.X, Y: p.Y, Score: p.Score,
			Energy: p.Energy, Territory: territory[p.ID],
			Connected: p.Connected, Eliminated: p.Eliminated,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func (e *Engine) Status() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]any{"match_id": e.matchID, "turn": e.turn, "max_turns": e.cfg.MaxTurns, "game_over": e.gameOver, "players": len(e.players), "max_players": e.cfg.MaxPlayers, "pending_actions": len(e.pending)}
}

func (e *Engine) Results() []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Result, len(e.results))
	copy(out, e.results)
	return out
}

func clonePlayer(p *Player) *Player { c := *p; return &c }
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// TurnSignal returns the current turn number and a channel that is closed
// when that turn resolves. The caller must call this again after the channel
// closes to get the next turn's signal channel.
func (e *Engine) TurnSignal() (int, <-chan struct{}) {
	e.mu.RLock()
	turn := e.turn
	e.mu.RUnlock()
	e.turnChangedMu.Lock()
	ch := e.turnChanged
	e.turnChangedMu.Unlock()
	return turn, ch
}

// WaitForTurn blocks until the engine's turn is greater than the given turn.
func (e *Engine) WaitForTurn(turn int, ctx context.Context) {
	for {
		e.mu.RLock()
		current := e.turn
		e.mu.RUnlock()
		if current > turn {
			return
		}
		e.turnChangedMu.Lock()
		ch := e.turnChanged
		e.turnChangedMu.Unlock()
		select {
		case <-ch:
			// Turn resolved, loop back to check.
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) IsGameOver() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.gameOver
}

func (e *Engine) Config() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

func (e *Engine) ResultsForTurn(turn int) []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Result, 0)
	for _, result := range e.results {
		if result.Turn == turn {
			out = append(out, result)
		}
	}
	return out
}

// Reset resets the game state to turn 0 while keeping all registered players.
// Cells are restored to their initial state, pending actions and results are cleared.
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.turn = 0
	e.gameOver = false
	e.pending = make(map[string]Action)
	e.results = nil
	e.roundStarted = time.Now()

	// Recreate turnChanged channel.
	e.turnChangedMu.Lock()
	close(e.turnChanged)
	e.turnChanged = make(chan struct{})
	e.turnChangedMu.Unlock()

	// Reset all cells to initial state.
	for y := 0; y < e.cfg.Height; y++ {
		for x := 0; x < e.cfg.Width; x++ {
			p := Point{x, y}
			cell := e.cells[p]
			cell.Owner = ""
			cell.Health = 3
		}
	}

	// Reset players: keep them, but reset scores.
	for _, p := range e.players {
		p.Score = 0
	}
}
