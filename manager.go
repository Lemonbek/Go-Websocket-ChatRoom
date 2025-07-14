package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
    EventVoteMessage = "vote_message"
)

var (
	webSocketUpgrader = websocket.Upgrader{
		CheckOrigin:     checkOrigin,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type Manager struct {
	client ClientList
	sync.RWMutex
	otps     RetentionMap
	handlers map[string]EventHandler
}

func NewManager(ctx context.Context) *Manager {
	return &Manager{
		client:   make(ClientList),
		handlers: make(map[string]EventHandler),
		otps:     NewRetentionMap(ctx, 5*time.Second),
	}
}

func (m *Manager) setupEventHandlers() {
	m.handlers[EventSendMessage] = SendMessage
	m.handlers["join_chatroom"] = JoinChatroomHandler
	m.handlers[EventVoteMessage] = VoteMessageHandler
}
func SendMessage(event Event, c *Client) error {
    // Unmarshal the payload to extract the message and chatroom
    var payload struct {
        Username string `json:"username"`
        Message  string `json:"message"`
        Chatroom string `json:"chatroom"`
    }
    if err := json.Unmarshal(event.Payload, &payload); err != nil {
        log.Println("invalid payload format", err)
        return err
    }
    // Persist the message to the chatroom's history file
    msg := map[string]interface{}{
        "id":       uuid.NewString(),
        "username": payload.Username,
        "message":  payload.Message,
        "chatroom": payload.Chatroom,
        "timestamp": time.Now().Format(time.RFC3339),
        "likes":    0,
        "dislikes": 0,
    }
    saveMessageToHistory(payload.Chatroom, msg)
    // Create a new event to broadcast (with id, likes, dislikes)
    broadcastEvent := Event{
        Type:    "new_message",
        Payload: mustMarshal(msg),
    }
    // Broadcast to all clients
    c.manager.RLock()
    defer c.manager.RUnlock()
    for client := range c.manager.client {
        select {
        case client.egress <- broadcastEvent:
        default:
            log.Println("client egress channel full, skipping")
        }
    }
    return nil
}
func (m *Manager) routeEvent(event Event, c *Client) error {
	if handler, ok := m.handlers[event.Type]; ok {
		if err := handler(event, c); err != nil {
			return err
		}
		return nil
	} else {
		return errors.New("there is no such event")
	}
}
func (m *Manager) serveWs(w http.ResponseWriter, r *http.Request) {
	otp := r.URL.Query().Get("otp")
	if otp == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !m.otps.VerifyOTP(otp) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Println("New Connection")
	//Upgrade Regular HTTP to Websocket
	conn, err := webSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := NewClient(conn, m)
	m.addClient(client)

	//start client messages
	go client.readMessages()
	go client.writeMessages()
}

func (m *Manager) loginHandler(w http.ResponseWriter, r *http.Request) {
	type userLoginRequest struct {
		UserName string `json:"username"`
		Password string `json:"password"`
	}
	var req userLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	if req.UserName == "chris" || req.UserName == "freddie" || req.UserName == "luke" && req.Password == "password" {
		type response struct {
			OTP string `json:"otp"`
		}
		otp := m.otps.NewOTP()
		resp := response{
			OTP: otp.Key,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Println(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
}

func (m *Manager) addClient(client *Client) {
	m.Lock()
	defer m.Unlock()
	m.client[client] = true
}

func (m *Manager) removeClient(client *Client) {
	m.Lock()
	defer m.Unlock()
	if _, ok := m.client[client]; ok {
		client.connection.Close()
		delete(m.client, client)
	}
	m.client[client] = false
}
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	switch origin {
	case "http://localhost:8080":
		return true
	default:
		return false
	}
}

func saveMessageToHistory(chatroom string, msg map[string]interface{}) {
    if chatroom == "" {
        chatroom = "general"
    }
    filename := "history_" + chatroom + ".json"
    f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        log.Println("failed to open history file:", err)
        return
    }
    defer f.Close()
    enc := json.NewEncoder(f)
    if err := enc.Encode(msg); err != nil {
        log.Println("failed to write message to history:", err)
    }
}

func JoinChatroomHandler(event Event, c *Client) error {
    var payload struct {
        Chatroom string `json:"chatroom"`
    }
    if err := json.Unmarshal(event.Payload, &payload); err != nil {
        log.Println("invalid join_chatroom payload format", err)
        return err
    }
    if payload.Chatroom == "" {
        payload.Chatroom = "general"
    }
    filename := "history_" + payload.Chatroom + ".json"
    f, err := os.Open(filename)
    if err != nil {
        if os.IsNotExist(err) {
            // No history yet, that's fine
            return nil
        }
        log.Println("failed to open history file:", err)
        return err
    }
    defer f.Close()
    dec := json.NewDecoder(f)
    for {
        var msg map[string]interface{}
        if err := dec.Decode(&msg); err != nil {
            break
        }
        // Send each message to the client
        historyEvent := Event{
            Type:    "new_message",
            Payload: mustMarshal(msg),
        }
        c.egress <- historyEvent
    }
    return nil
}
func mustMarshal(v interface{}) json.RawMessage {
    b, _ := json.Marshal(v)
    return b
}

func VoteMessageHandler(event Event, c *Client) error {
    var payload struct {
        MessageID string `json:"id"`
        Chatroom  string `json:"chatroom"`
        VoteType  string `json:"voteType"` // "up" or "down"
    }
    if err := json.Unmarshal(event.Payload, &payload); err != nil {
        log.Println("invalid vote_message payload format", err)
        return err
    }
    if payload.Chatroom == "" {
        payload.Chatroom = "general"
    }
    filename := "history_" + payload.Chatroom + ".json"
    // Read all messages
    f, err := os.Open(filename)
    if err != nil {
        log.Println("failed to open history file for voting:", err)
        return err
    }
    defer f.Close()
    var messages []map[string]interface{}
    dec := json.NewDecoder(f)
    for {
        var msg map[string]interface{}
        if err := dec.Decode(&msg); err != nil {
            break
        }
        messages = append(messages, msg)
    }
    // Find and update the message
    updated := false
    for _, msg := range messages {
        if msg["id"] == payload.MessageID {
            if payload.VoteType == "up" {
                if v, ok := msg["likes"].(float64); ok {
                    msg["likes"] = v + 1
                } else {
                    msg["likes"] = 1
                }
            } else if payload.VoteType == "down" {
                if v, ok := msg["dislikes"].(float64); ok {
                    msg["dislikes"] = v + 1
                } else {
                    msg["dislikes"] = 1
                }
            }
            updated = true
            // Broadcast updated message
            broadcastEvent := Event{
                Type:    "new_message",
                Payload: mustMarshal(msg),
            }
            c.manager.RLock()
            for client := range c.manager.client {
                select {
                case client.egress <- broadcastEvent:
                default:
                    log.Println("client egress channel full, skipping")
                }
            }
            c.manager.RUnlock()
            break
        }
    }
    if !updated {
        log.Println("message to vote not found")
        return nil
    }
    // Rewrite the file with updated messages
    f2, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
    if err != nil {
        log.Println("failed to open history file for rewrite:", err)
        return err
    }
    defer f2.Close()
    enc := json.NewEncoder(f2)
    for _, msg := range messages {
        if err := enc.Encode(msg); err != nil {
            log.Println("failed to write message to history:", err)
        }
    }
    return nil
}
