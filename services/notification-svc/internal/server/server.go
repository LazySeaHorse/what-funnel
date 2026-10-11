package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// SessionStore loads the whole session in one lookup (see go-common/session.Reader).
type SessionStore interface {
	GetSession(r *http.Request) (map[string]string, error)
}

// sessionIdentity is who an authenticated session belongs to.
type sessionIdentity struct {
	userID, accountID uuid.UUID
	role              string
}

// identify loads the session once. The error text is the client-facing 401 reason.
func (s *Server) identify(r *http.Request) (sessionIdentity, error) {
	data, err := s.sess.GetSession(r)
	if err != nil {
		return sessionIdentity{}, errors.New("unauthenticated")
	}
	var id sessionIdentity
	if id.userID, err = uuid.Parse(data["user_id"]); err != nil {
		return sessionIdentity{}, errors.New("unauthenticated")
	}
	if id.accountID, err = uuid.Parse(data["account_id"]); err != nil {
		return sessionIdentity{}, errors.New("unauthenticated: missing account")
	}
	var ok bool
	if id.role, ok = data["role"]; !ok {
		return sessionIdentity{}, errors.New("unauthenticated: missing role")
	}
	return id, nil
}

type Client struct {
	UserID    uuid.UUID
	AccountID uuid.UUID
	Role      string
	Conn      *websocket.Conn
	Send      chan []byte
	Hub       *Hub
	// sessionReq carries the cookies of the upgrade request so the session can
	// be re-validated against the store while the socket stays open.
	sessionReq *http.Request
}

type Hub struct {
	clients map[*Client]struct{}
	closed  bool
	mu      sync.RWMutex
	logger  *slog.Logger
}

var ErrHubClosed = errors.New("websocket hub is closed")

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		logger:  logger,
	}
}

func (h *Hub) RegisterClient(client *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return ErrHubClosed
	}
	h.clients[client] = struct{}{}
	h.logger.Debug("client registered", "user_id", client.UserID)
	return nil
}

func (h *Hub) UnregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; !ok {
		return
	}
	delete(h.clients, client)
	close(client.Send)
	h.logger.Debug("client unregistered", "user_id", client.UserID)
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}
	h.closed = true
	for client := range h.clients {
		delete(h.clients, client)
		close(client.Send)
	}
}

func (h *Hub) BroadcastToAccount(accountID uuid.UUID, event any, filterFunc func(userID uuid.UUID, role string) bool) {
	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("failed to marshal websocket event", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.AccountID == accountID {
			if filterFunc == nil || filterFunc(client.UserID, client.Role) {
				select {
				case client.Send <- data:
				default:
					h.logger.Warn("client send channel blocked", "user_id", client.UserID)
				}
			}
		}
	}
}

type Server struct {
	hub            *Hub
	sess           SessionStore
	logger         *slog.Logger
	upgrader       websocket.Upgrader
	allowedOrigins []string
	isProd         bool
}

func NewServer(hub *Hub, sess SessionStore, logger *slog.Logger, allowedOrigins []string, isProd bool) *Server {
	s := &Server{
		hub:            hub,
		sess:           sess,
		logger:         logger,
		allowedOrigins: allowedOrigins,
		isProd:         isProd,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     s.CheckOrigin,
	}
	return s
}

// CheckOrigin validates the incoming WebSocket upgrade request against the origin whitelist.
func (s *Server) CheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser or direct clients without Origin header
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return middleware.OriginAllowed(u, r, s.allowedOrigins, s.isProd)
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	// 1. Session Auth
	id, err := s.identify(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// 2. Upgrade to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade websocket connection", "error", err)
		return
	}

	client := &Client{
		UserID:    id.userID,
		AccountID: id.accountID,
		Role:      id.role,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Hub:       s.hub,
		// Detach from the request context, which ends once the handler returns.
		sessionReq: r.Clone(context.Background()),
	}

	if err := s.hub.RegisterClient(client); err != nil {
		_ = conn.Close()
		return
	}

	// Start loops
	go client.writePump()
	go client.readPump()
}

// DefaultRevalidateInterval is how often open sockets re-check their session.
const DefaultRevalidateInterval = 45 * time.Second

// RunRevalidation periodically re-validates every connected client's session
// against the store until ctx is done. Sockets whose session no longer exists
// (logout, revocation, expiry) are closed; sockets whose role changed pick up
// the new role.
func (s *Server) RunRevalidation(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRevalidateInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RevalidateClients()
		}
	}
}

// RevalidateClients performs one revalidation pass over all connected clients.
func (s *Server) RevalidateClients() {
	for _, client := range s.hub.snapshotClients() {
		if client.sessionReq == nil {
			continue
		}
		id, err := s.identify(client.sessionReq)
		if err != nil || id.userID != client.UserID || id.accountID != client.AccountID {
			s.logger.Info("closing websocket: session no longer valid", "user_id", client.UserID)
			s.hub.UnregisterClient(client)
			continue
		}
		s.hub.updateRole(client, id.role)
	}
}

func (h *Hub) snapshotClients() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		out = append(out, c)
	}
	return out
}

func (h *Hub) updateRole(client *Client, role string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[client]; ok && client.Role != role {
		h.logger.Info("websocket client role changed", "user_id", client.UserID, "role", role)
		client.Role = role
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.UnregisterClient(c)
		_ = c.Conn.Close()
	}()
	c.Conn.SetReadLimit(512)
	_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// Websocket outbound payload types (matching specs)
type WSMessageEvent struct {
	Type           string         `json:"type"`
	ConversationID string         `json:"conversation_id"`
	Message        *types.Message `json:"message"`
}

type WSConversationAssignedEvent struct {
	Type            string   `json:"type"`
	ConversationID  string   `json:"conversation_id"`
	AssignedUserIDs []string `json:"assigned_user_ids"`
}

type WSChannelStatusChangedEvent struct {
	Type      string `json:"type"`
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
}

type WSLeadStateChangedEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	LeadID         string `json:"lead_id"`
	FromState      string `json:"from_state"`
	ToState        string `json:"to_state"`
}

type WSAutomationSuggestionCreatedEvent struct {
	Type         string          `json:"type"`
	SuggestionID string          `json:"suggestion_id"`
	Payload      json.RawMessage `json:"payload"`
}

type WSAIReplyReadyEvent struct {
	Type           string   `json:"type"`
	ConversationID string   `json:"conversation_id"`
	MessageID      string   `json:"message_id"`
	DraftID        string   `json:"draft_id"`
	DraftText      string   `json:"draft_text"`
	StageMatched   string   `json:"stage_matched"`
	Confidence     *float64 `json:"confidence"`
}
