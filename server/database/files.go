package database

import (
	"context"
	"fmt"
	"time"

	"sOPown3d/internal/agent/transfer"
	"sOPown3d/server/logger"
)

// SaveFileMetadata stores file metadata in the database
func (db *DB) SaveFileMetadata(metadata transfer.FileMetadata, storedPath string) error {
	query := `
        INSERT INTO transferred_files (
            agent_id, original_path, filename, stored_path,
            size, checksum, mod_time, permissions, uploaded_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Pool.Exec(ctx, query,
		metadata.AgentID,
		metadata.OriginalPath,
		metadata.Filename,
		storedPath,
		metadata.Size,
		metadata.Checksum,
		time.Unix(metadata.ModTime, 0),
		metadata.Permissions,
	)

	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to save file metadata: %v", err)
		return fmt.Errorf("failed to save file metadata: %w", err)
	}

	db.logger.Info(logger.CategoryStorage, "Saved file metadata: %s (agent: %s)", metadata.Filename, metadata.AgentID)
	return nil
}

// GetFilesByAgent retrieves files for a specific agent
func (db *DB) GetFilesByAgent(agentID string, limit int) ([]FileRecord, error) {
	query := `
        SELECT id, agent_id, original_path, filename, stored_path,
               size, checksum, mod_time, permissions, uploaded_at
        FROM transferred_files
        WHERE agent_id = $1
        ORDER BY uploaded_at DESC
        LIMIT $2
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Pool.Query(ctx, query, agentID, limit)
	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to query files: %v", err)
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		err := rows.Scan(
			&f.ID,
			&f.AgentID,
			&f.OriginalPath,
			&f.Filename,
			&f.StoredPath,
			&f.Size,
			&f.Checksum,
			&f.ModTime,
			&f.Permissions,
			&f.UploadedAt,
		)
		if err != nil {
			db.logger.Error(logger.CategoryDatabase, "Failed to scan row: %v", err)
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		files = append(files, f)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	db.logger.Info(logger.CategoryDatabase, "Retrieved %d files for agent %s", len(files), agentID)
	return files, nil
}

// GetFileByID retrieves a specific file record
func (db *DB) GetFileByID(fileID string) (*FileRecord, error) {
	query := `
        SELECT id, agent_id, original_path, filename, stored_path,
               size, checksum, mod_time, permissions, uploaded_at
        FROM transferred_files
        WHERE id = $1
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var f FileRecord
	err := db.Pool.QueryRow(ctx, query, fileID).Scan(
		&f.ID,
		&f.AgentID,
		&f.OriginalPath,
		&f.Filename,
		&f.StoredPath,
		&f.Size,
		&f.Checksum,
		&f.ModTime,
		&f.Permissions,
		&f.UploadedAt,
	)

	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to get file %s: %v", fileID, err)
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	return &f, nil
}

// GetRecentFiles retrieves most recent files across all agents
func (db *DB) GetRecentFiles(limit int) ([]FileRecord, error) {
	query := `
        SELECT id, agent_id, original_path, filename, stored_path,
               size, checksum, mod_time, permissions, uploaded_at
        FROM transferred_files
        ORDER BY uploaded_at DESC
        LIMIT $1
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Pool.Query(ctx, query, limit)
	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to query recent files: %v", err)
		return nil, fmt.Errorf("failed to query recent files: %w", err)
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		err := rows.Scan(
			&f.ID,
			&f.AgentID,
			&f.OriginalPath,
			&f.Filename,
			&f.StoredPath,
			&f.Size,
			&f.Checksum,
			&f.ModTime,
			&f.Permissions,
			&f.UploadedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		files = append(files, f)
	}

	return files, rows.Err()
}

// DeleteFile removes a file record from the database
func (db *DB) DeleteFile(fileID string) error {
	query := `DELETE FROM transferred_files WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := db.Pool.Exec(ctx, query, fileID)
	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to delete file %s: %v", fileID, err)
		return fmt.Errorf("failed to delete file: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("file not found: %s", fileID)
	}

	db.logger.Info(logger.CategoryCleanup, "Deleted file %s", fileID)
	return nil
}
