package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins in dev
		return true
	},
}

type SessionStore interface {
	GetUserID(r *http.Request) (uuid.UUID, bool)
	GetAccountID(r *http.Request) (uuid.UUID, bool)
	GetRole(r *http.Request) (string, bool)
}

type Client struct {
	UserID    uuid.UUID
	AccountID uuid.UUID
	Role      string
	Conn      *websocket.Conn
	Send      chan []byte
	Hub       *Hub
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	logger     *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		logger:     logger,
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("client registered", "user_id", client.UserID)
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				h.logger.Debug("client unregistered", "user_id", client.UserID)
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) RegisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = true
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
	hub    *Hub
	sess   SessionStore
	logger *slog.Logger
}

func NewServer(hub *Hub, sess SessionStore, logger *slog.Logger) *Server {
	return &Server{
		hub:    hub,
		sess:   sess,
		logger: logger,
	}
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	// 1. Session Auth
	userID, ok := s.sess.GetUserID(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthenticated"}`))
		return
	}
	accountID, ok := s.sess.GetAccountID(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthenticated: missing account"}`))
		return
	}
	role, ok := s.sess.GetRole(r)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthenticated: missing role"}`))
		return
	}

	// 2. Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade websocket connection", "error", err)
		return
	}

	client := &Client{
		UserID:    userID,
		AccountID: accountID,
		Role:      role,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Hub:       s.hub,
	}

	s.hub.register <- client

	// Start loops
	go client.writePump()
	go client.readPump()
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
		c.Hub.unregister <- c
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
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	AnswerMarkdown string `json:"answer_markdown"`
}
