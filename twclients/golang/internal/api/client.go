package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"twclient/internal/models"
)

// Client communicates with the Territory Wars game server via HTTP and
// WebSocket. It handles registration, observation polling, action submission,
// and leaderboard queries.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	wsURL      string
	playerName string
	logger     *slog.Logger
}

// New creates a Client ready to interact with the server.
// baseURL is the full server URL (e.g. "http://localhost:8080").
// token is the authentication token obtained from Join.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		token:  token,
		logger: slog.Default(),
	}
}

// NewWithLogger creates a Client with a custom logger.
func NewWithLogger(baseURL, token string, logger *slog.Logger) *Client {
	c := New(baseURL, token)
	c.logger = logger
	return c
}

// Join registers a new player with the server.
func (c *Client) Join(ctx context.Context, name string) (*models.JoinResponse, error) {
	payload, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("marshaling join request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/join", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating join request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("join request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("join failed with status %d: %s", resp.StatusCode, string(body))
	}

	var joinResp models.JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&joinResp); err != nil {
		return nil, fmt.Errorf("decoding join response: %w", err)
	}

	c.token = joinResp.Token
	c.wsURL = joinResp.WebsocketURL
	c.playerName = name

	return &joinResp, nil
}

// GetObservation fetches the current game state for the authenticated player.
func (c *Client) GetObservation(ctx context.Context) (*models.Observation, error) {
	url := fmt.Sprintf("%s/api/v1/state", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating observation request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("observation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("unauthorized: check your token")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("observation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var obs models.Observation
	if err := json.NewDecoder(resp.Body).Decode(&obs); err != nil {
		return nil, fmt.Errorf("decoding observation: %w", err)
	}

	return &obs, nil
}

// SubmitAction sends an action message to the server via HTTP.
// This is a fallback for when WebSocket is unavailable.
func (c *Client) SubmitAction(ctx context.Context, msg models.ActionMessage) error {
	url := fmt.Sprintf("%s/api/v1/action", c.baseURL)
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling action: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating action request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("action request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("action failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SubmitActionWebSocket sends an action message via the WebSocket connection.
func (c *Client) SubmitActionWebSocket(conn *websocket.Conn, msg models.ActionMessage) error {
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("writing action to WebSocket: %w", err)
	}
	return nil
}

// GetLeaderboard fetches the current leaderboard and game status.
func (c *Client) GetLeaderboard(ctx context.Context) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/leaderboard", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating leaderboard request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("leaderboard request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("leaderboard failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding leaderboard: %w", err)
	}

	return result, nil
}

// ConnectWebSocket establishes a WebSocket connection to the game server.
// The token is passed as a query parameter.
func (c *Client) ConnectWebSocket(ctx context.Context) (*websocket.Conn, error) {
	wsURL := c.wsURL
	if wsURL == "" {
		wsURL = "/ws"
	}

	// Build the full WebSocket URL.
	fullURL := c.baseURL
	// Ensure baseURL doesn't end with /
	for len(fullURL) > 0 && fullURL[len(fullURL)-1] == '/' {
		fullURL = fullURL[:len(fullURL)-1]
	}
	// Ensure wsURL starts with /
	for len(wsURL) > 0 && wsURL[0] == '/' {
		wsURL = wsURL[1:]
	}
	fullURL += "/" + wsURL

	// Add token as query parameter.
	fullURL += "?token=" + c.token

	// Convert http:// to ws://
	var wsFullURL string
	if strings.HasPrefix(fullURL, "https://") {
		wsFullURL = "wss://" + fullURL[len("https://"):]
	} else {
		wsFullURL = "ws://" + fullURL[len("http://"):]
	}

	c.logger.Info("WebSocket URL", "url", wsFullURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsFullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("WebSocket dial failed: %w", err)
	}

	// Respond to server pings with pongs so the server doesn't
	// close the connection due to read deadline expiry.
	conn.SetPingHandler(func(message string) error {
		return conn.WriteMessage(websocket.PongMessage, []byte(message))
	})

	return conn, nil
}

// BuildActionMessage creates an ActionMessage from the current turn and an action.
// If action is nil, it creates a "pass" action (no target).
func BuildActionMessage(turn int, action *models.Action) models.ActionMessage {
	var actions []models.Action
	if action == nil {
		actions = []models.Action{{Type: "pass"}}
	} else {
		actions = []models.Action{*action}
	}
	return models.ActionMessage{
		Type:    "ACTION",
		Turn:    turn,
		Actions: actions,
	}
}

// Close releases any resources held by the client.
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}

// BaseURL returns the server base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// PlayerName returns the player name (used for re-joining).
func (c *Client) PlayerName() string {
	return c.playerName
}

// UpdateToken sets a new authentication token.
func (c *Client) UpdateToken(token string) {
	c.token = token
}

// UpdateWSURL sets a new WebSocket URL.
func (c *Client) UpdateWSURL(wsURL string) {
	c.wsURL = wsURL
}
