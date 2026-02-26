package database

import (
	"github.com/google/uuid"
	"time"
)

// FileRecord represents a file stored in the database
type FileRecord struct {
	ID           string    `db:"id" json:"id"`
	AgentID      string    `db:"agent_id" json:"agent_id"`
	OriginalPath string    `db:"original_path" json:"original_path"`
	Filename     string    `db:"filename" json:"filename"`
	StoredPath   string    `db:"stored_path" json:"stored_path"`
	Size         int64     `db:"size" json:"size"`
	Checksum     string    `db:"checksum" json:"checksum"`
	ModTime      time.Time `db:"mod_time" json:"mod_time"`
	Permissions  string    `db:"permissions" json:"permissions"`
	UploadedAt   time.Time `db:"uploaded_at" json:"uploaded_at"`
}
type FileSearchCommand struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	AgentID     string     `db:"agent_id" json:"agent_id"`
	Pattern     string     `db:"pattern" json:"pattern"`
	SearchPaths []string   `db:"search_paths" json:"search_paths"`
	Extensions  []string   `db:"extensions" json:"extensions"`
	MaxDepth    int        `db:"max_depth" json:"max_depth"`
	Status      string     `db:"status" json:"status"`
	ResultCount int        `db:"result_count" json:"result_count"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

type FileSearchResult struct {
	ID              uuid.UUID `db:"id" json:"id"`
	SearchCommandID uuid.UUID `db:"search_command_id" json:"search_command_id"`
	FilePath        string    `db:"file_path" json:"path"`
	FileName        string    `db:"file_name" json:"name"`
	FileSize        int64     `db:"file_size" json:"size"`
	ModTime         time.Time `db:"mod_time" json:"mod_time"`
	Permissions     string    `db:"permissions" json:"permissions"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}

type FileListCommand struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	AgentID     string     `db:"agent_id" json:"agent_id"`
	Path        string     `db:"path" json:"path"`
	Status      string     `db:"status" json:"status"`
	ResultCount int        `db:"result_count" json:"result_count"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}

type FileListResult struct {
	ID            uuid.UUID `db:"id" json:"id"`
	ListCommandID uuid.UUID `db:"list_command_id" json:"list_command_id"`
	FilePath      string    `db:"file_path" json:"path"`
	FileName      string    `db:"file_name" json:"name"`
	FileSize      int64     `db:"file_size" json:"size"`
	ModTime       time.Time `db:"mod_time" json:"mod_time"`
	Permissions   string    `db:"permissions" json:"permissions"`
	IsDir         bool      `db:"is_dir" json:"is_dir"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// FileTransferCommand represents a file transfer request
type FileTransferCommand struct {
	ID                uuid.UUID  `db:"id" json:"id"`
	AgentID           string     `db:"agent_id" json:"agent_id"`
	FilePath          string     `db:"file_path" json:"file_path"`
	Status            string     `db:"status" json:"status"`
	TransferredFileID *uuid.UUID `db:"transferred_file_id" json:"transferred_file_id,omitempty"`
	CreatedAt         time.Time  `db:"created_at" json:"created_at"`
	CompletedAt       *time.Time `db:"completed_at" json:"completed_at,omitempty"`
}
