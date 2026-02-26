package handlers

import (
  "crypto/sha256"
  "encoding/base64"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "net/http"
  "os"
  "path/filepath"
  "strings"
  "time"

  "github.com/google/uuid"

  "sOPown3d/internal/agent/crypto"
  "sOPown3d/internal/agent/transfer"
  "sOPown3d/server/database"
  "sOPown3d/server/logger"
)

type FileHandler struct {
  storagePath string
  db          *database.DB
  logger      *logger.Logger
}

func NewFileHandler(storagePath string, db *database.DB, log *logger.Logger) *FileHandler {
  return &FileHandler{
    storagePath: storagePath,
    db:          db,
    logger:      log,
  }
}

// ──────────────────────────────────────────────
// Upload (agent → server)
// ──────────────────────────────────────────────

func (h *FileHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  agentID := r.Header.Get("X-Agent-ID")
  if agentID == "" {
    h.logger.Warn(logger.CategorySecurity, "Upload attempt without agent ID")
    http.Error(w, "missing agent ID", http.StatusBadRequest)
    return
  }

  if err := r.ParseMultipartForm(100 << 20); err != nil {
    h.logger.Error(logger.CategoryServer, "Failed to parse form: %v", err)
    http.Error(w, "failed to parse form", http.StatusBadRequest)
    return
  }

  // Decrypt metadata
  encryptedMetadata := r.FormValue("metadata")
  if encryptedMetadata == "" {
    http.Error(w, "missing metadata", http.StatusBadRequest)
    return
  }

  decryptedMetadataStr, err := crypto.Decrypt(encryptedMetadata)
  if err != nil {
    h.logger.Error(logger.CategorySecurity, "Metadata decryption failed from agent %s: %v", agentID, err)
    http.Error(w, "metadata decryption failed", http.StatusBadRequest)
    return
  }

  var metadata transfer.FileMetadata
  if err := json.Unmarshal([]byte(decryptedMetadataStr), &metadata); err != nil {
    h.logger.Error(logger.CategoryServer, "Invalid metadata format: %v", err)
    http.Error(w, "invalid metadata format", http.StatusBadRequest)
    return
  }

  // Decrypt file
  encryptedFile := r.FormValue("file")
  if encryptedFile == "" {
    http.Error(w, "missing file", http.StatusBadRequest)
    return
  }

  decryptedFileB64, err := crypto.Decrypt(encryptedFile)
  if err != nil {
    h.logger.Error(logger.CategorySecurity, "File decryption failed from agent %s: %v", agentID, err)
    http.Error(w, "file decryption failed", http.StatusBadRequest)
    return
  }

  decryptedFile, err := base64.StdEncoding.DecodeString(decryptedFileB64)
  if err != nil {
    h.logger.Error(logger.CategoryServer, "Failed to decode file: %v", err)
    http.Error(w, "failed to decode file", http.StatusBadRequest)
    return
  }

  // Verify checksum
  hash := sha256.Sum256(decryptedFile)
  checksum := hex.EncodeToString(hash[:])
  if checksum != metadata.Checksum {
    h.logger.Warn(logger.CategorySecurity, "Checksum mismatch for file %s from agent %s", metadata.Filename, agentID)
    http.Error(w, "checksum mismatch - file corrupted", http.StatusBadRequest)
    return
  }

  // Save to disk
  agentDir := filepath.Join(h.storagePath, agentID)
  if err := os.MkdirAll(agentDir, 0755); err != nil {
    h.logger.Error(logger.CategoryStorage, "Failed to create storage directory: %v", err)
    http.Error(w, "failed to create storage directory", http.StatusInternalServerError)
    return
  }

  timestamp := time.Now().Format("20060102_150405")
  safeName := fmt.Sprintf("%s_%s", timestamp, metadata.Filename)
  savePath := filepath.Join(agentDir, safeName)

  if err := os.WriteFile(savePath, decryptedFile, 0644); err != nil {
    h.logger.Error(logger.CategoryStorage, "Failed to save file: %v", err)
    http.Error(w, "failed to save file", http.StatusInternalServerError)
    return
  }

  // Save metadata to DB
  if err := h.db.SaveFileMetadata(metadata, savePath); err != nil {
    h.logger.Error(logger.CategoryDatabase, "File saved but metadata storage failed: %v", err)
    http.Error(w, "file saved but metadata storage failed", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "File uploaded: %s (agent: %s, size: %d bytes)",
    metadata.Filename, agentID, len(decryptedFile))

  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(http.StatusOK)
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status":      "success",
    "stored_path": savePath,
    "filename":    safeName,
    "size":        len(decryptedFile),
  })
}

// ──────────────────────────────────────────────
// File listing
// ──────────────────────────────────────────────

func (h *FileHandler) GetAgentFiles(w http.ResponseWriter, r *http.Request) {
  agentID := r.URL.Query().Get("agent_id")
  if agentID == "" {
    http.Error(w, "missing agent_id parameter", http.StatusBadRequest)
    return
  }

  files, err := h.db.GetFilesByAgent(agentID, 100)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get files for agent %s: %v", agentID, err)
    http.Error(w, "failed to retrieve files", http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(files)
}

func (h *FileHandler) GetRecentFiles(w http.ResponseWriter, r *http.Request) {
  files, err := h.db.GetRecentFiles(50)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get recent files: %v", err)
    http.Error(w, "failed to retrieve files", http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(files)
}

// ──────────────────────────────────────────────
// File search
// ──────────────────────────────────────────────

func (h *FileHandler) SearchFiles(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    AgentID     string   `json:"agent_id"`
    Pattern     string   `json:"pattern"`
    SearchPaths []string `json:"search_paths"`
    Extensions  []string `json:"extensions"`
    MaxDepth    int      `json:"max_depth"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  if req.AgentID == "" {
    http.Error(w, "missing agent_id", http.StatusBadRequest)
    return
  }

  if req.MaxDepth == 0 {
    req.MaxDepth = 5
  }

  cmd := &database.FileSearchCommand{
    AgentID:     req.AgentID,
    Pattern:     req.Pattern,
    SearchPaths: req.SearchPaths,
    Extensions:  req.Extensions,
    MaxDepth:    req.MaxDepth,
  }

  if err := h.db.CreateFileSearchCommand(cmd); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to create search command: %v", err)
    http.Error(w, "failed to create search command", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "Search command queued: %s (agent: %s, pattern: %s)",
    cmd.ID, req.AgentID, req.Pattern)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status":     "queued",
    "command_id": cmd.ID.String(),
  })
}

// ──────────────────────────────────────────────
// File list (directory browsing)
// ──────────────────────────────────────────────

func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    AgentID string `json:"agent_id"`
    Path    string `json:"path"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  if req.AgentID == "" {
    http.Error(w, "missing agent_id", http.StatusBadRequest)
    return
  }

  cmd := &database.FileListCommand{
    AgentID: req.AgentID,
    Path:    req.Path,
  }

  if err := h.db.CreateFileListCommand(cmd); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to create list command: %v", err)
    http.Error(w, "failed to create list command", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "List command queued: %s (agent: %s, path: %s)",
    cmd.ID, req.AgentID, req.Path)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status":     "queued",
    "command_id": cmd.ID.String(),
  })
}

func (h *FileHandler) GetSearchResults(w http.ResponseWriter, r *http.Request) {
  // URL: /api/files/search-results/{uuid}
  commandIDStr := strings.TrimPrefix(r.URL.Path, "/api/files/search-results/")
  if commandIDStr == "" {
    http.Error(w, "missing command ID", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(commandIDStr)
  if err != nil {
    http.Error(w, "invalid command ID format", http.StatusBadRequest)
    return
  }

  cmd, err := h.db.GetFileSearchCommand(commandID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get search command: %v", err)
    http.Error(w, "search command not found", http.StatusNotFound)
    return
  }

  results, err := h.db.GetFileSearchResults(commandID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get search results: %v", err)
    http.Error(w, "failed to retrieve results", http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "command_id":   cmd.ID.String(),
    "status":       cmd.Status,
    "result_count": len(results),
    "results":      results,
  })
}

func (h *FileHandler) GetListResults(w http.ResponseWriter, r *http.Request) {
  // URL: /api/files/list-results/{uuid}
  commandIDStr := strings.TrimPrefix(r.URL.Path, "/api/files/list-results/")
  if commandIDStr == "" {
    http.Error(w, "missing command ID", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(commandIDStr)
  if err != nil {
    http.Error(w, "invalid command ID format", http.StatusBadRequest)
    return
  }

  cmd, err := h.db.GetFileListCommand(commandID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get list command: %v", err)
    http.Error(w, "list command not found", http.StatusNotFound)
    return
  }

  results, err := h.db.GetFileListResults(commandID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get list results: %v", err)
    http.Error(w, "failed to retrieve results", http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "command_id":   cmd.ID.String(),
    "status":       cmd.Status,
    "result_count": len(results),
    "path":         cmd.Path,
    "results":      results,
  })
}

// ──────────────────────────────────────────────
// File transfer
// ──────────────────────────────────────────────

func (h *FileHandler) TransferFile(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    AgentID  string `json:"agent_id"`
    FilePath string `json:"file_path"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  if req.AgentID == "" || req.FilePath == "" {
    http.Error(w, "missing agent_id or file_path", http.StatusBadRequest)
    return
  }

  cmd := &database.FileTransferCommand{
    AgentID:  req.AgentID,
    FilePath: req.FilePath,
  }

  if err := h.db.CreateFileTransferCommand(cmd); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to create transfer command: %v", err)
    http.Error(w, "failed to create transfer command", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "Transfer command queued: %s (agent: %s, file: %s)",
    cmd.ID, req.AgentID, req.FilePath)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status":     "queued",
    "command_id": cmd.ID.String(),
  })
}

func (h *FileHandler) GetTransferStatus(w http.ResponseWriter, r *http.Request) {
  // URL: /api/files/transfer-status/{uuid}
  commandIDStr := strings.TrimPrefix(r.URL.Path, "/api/files/transfer-status/")
  if commandIDStr == "" {
    http.Error(w, "missing command ID", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(commandIDStr)
  if err != nil {
    http.Error(w, "invalid command ID format", http.StatusBadRequest)
    return
  }

  cmd, err := h.db.GetFileTransferCommand(commandID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get transfer command: %v", err)
    http.Error(w, "transfer command not found", http.StatusNotFound)
    return
  }

  response := map[string]interface{}{
    "command_id": cmd.ID.String(),
    "status":     cmd.Status,
    "file_path":  cmd.FilePath,
    "created_at": cmd.CreatedAt,
  }

  if cmd.TransferredFileID != nil {
    response["transferred_file_id"] = cmd.TransferredFileID.String()
  }
  if cmd.CompletedAt != nil {
    response["completed_at"] = *cmd.CompletedAt
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(response)
}

// ──────────────────────────────────────────────
// Agent polling endpoint
// ──────────────────────────────────────────────

func (h *FileHandler) GetPendingCommands(w http.ResponseWriter, r *http.Request) {
  agentID := r.URL.Query().Get("agent_id")
  if agentID == "" {
    agentID = r.Header.Get("X-Agent-ID")
  }
  if agentID == "" {
    http.Error(w, "missing agent_id", http.StatusBadRequest)
    return
  }

  commands, err := h.db.GetPendingFileCommands(agentID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to get pending commands for agent %s: %v", agentID, err)
    http.Error(w, "failed to retrieve commands", http.StatusInternalServerError)
    return
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "commands": commands,
  })
}

// ──────────────────────────────────────────────
// Agent submits search results back
// ──────────────────────────────────────────────

func (h *FileHandler) SubmitSearchResults(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    CommandID string `json:"command_id"`
    Results   []struct {
      FilePath    string    `json:"file_path"`
      FileName    string    `json:"file_name"`
      FileSize    int64     `json:"file_size"`
      ModTime     time.Time `json:"mod_time"`
      Permissions string    `json:"permissions"`
    } `json:"results"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(req.CommandID)
  if err != nil {
    http.Error(w, "invalid command_id format", http.StatusBadRequest)
    return
  }

  // Convert to database model
  var dbResults []database.FileSearchResult
  for _, r := range req.Results {
    dbResults = append(dbResults, database.FileSearchResult{
      FilePath:    r.FilePath,
      FileName:    r.FileName,
      FileSize:    r.FileSize,
      ModTime:     r.ModTime,
      Permissions: r.Permissions,
    })
  }

  if err := h.db.SaveFileSearchResults(commandID, dbResults); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to save search results: %v", err)
    http.Error(w, "failed to save results", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "Search results saved: %d files (command: %s)", len(dbResults), commandID)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status": "saved",
    "count":  len(dbResults),
  })
}

// UpdateTransferStatus receives transfer completion from agent
func (h *FileHandler) UpdateTransferStatus(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    CommandID string `json:"command_id"`
    Status    string `json:"status"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(req.CommandID)
  if err != nil {
    http.Error(w, "invalid command_id", http.StatusBadRequest)
    return
  }

  if err := h.db.UpdateTransferCommandStatus(commandID, req.Status, nil); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to update transfer status: %v", err)
    http.Error(w, "failed to update status", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "Transfer %s status: %s", req.CommandID, req.Status)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// ──────────────────────────────────────────────
// Download file from server storage
// ──────────────────────────────────────────────

func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
  fileID := strings.TrimPrefix(r.URL.Path, "/api/files/download/")
  if fileID == "" {
    http.Error(w, "missing file ID", http.StatusBadRequest)
    return
  }

  record, err := h.db.GetFileByID(fileID)
  if err != nil {
    h.logger.Error(logger.CategoryDatabase, "File not found: %v", err)
    http.Error(w, "file not found", http.StatusNotFound)
    return
  }

  // Check file exists on disk
  if _, err := os.Stat(record.StoredPath); os.IsNotExist(err) {
    http.Error(w, "file missing from storage", http.StatusNotFound)
    return
  }

  w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, record.Filename))
  w.Header().Set("Content-Type", "application/octet-stream")
  http.ServeFile(w, r, record.StoredPath)
}

// ──────────────────────────────────────────────
// Agent submits list results back
// ──────────────────────────────────────────────

func (h *FileHandler) SubmitListResults(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    return
  }

  var req struct {
    CommandID string `json:"command_id"`
    Results   []struct {
      FilePath    string    `json:"file_path"`
      FileName    string    `json:"file_name"`
      FileSize    int64     `json:"file_size"`
      ModTime     time.Time `json:"mod_time"`
      Permissions string    `json:"permissions"`
      IsDir       bool      `json:"is_dir"`
    } `json:"results"`
  }

  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
  }

  commandID, err := uuid.Parse(req.CommandID)
  if err != nil {
    http.Error(w, "invalid command_id format", http.StatusBadRequest)
    return
  }

  var dbResults []database.FileListResult
  for _, r := range req.Results {
    dbResults = append(dbResults, database.FileListResult{
      FilePath:    r.FilePath,
      FileName:    r.FileName,
      FileSize:    r.FileSize,
      ModTime:     r.ModTime,
      Permissions: r.Permissions,
      IsDir:       r.IsDir,
    })
  }

  if err := h.db.SaveFileListResults(commandID, dbResults); err != nil {
    h.logger.Error(logger.CategoryDatabase, "Failed to save list results: %v", err)
    http.Error(w, "failed to save results", http.StatusInternalServerError)
    return
  }

  h.logger.Info(logger.CategorySuccess, "List results saved: %d entries (command: %s)", len(dbResults), commandID)

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]interface{}{
    "status": "saved",
    "count":  len(dbResults),
  })
}
