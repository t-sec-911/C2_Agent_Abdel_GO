package database

import (
	"context"
	"fmt"
	"sOPown3d/server/logger"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateFileSearchCommand creates a new file search command
func (db *DB) CreateFileSearchCommand(cmd *FileSearchCommand) error {
	query := `
        INSERT INTO file_search_commands (
            id, agent_id, pattern, search_paths, extensions, max_depth, status
        ) VALUES ($1, $2, $3, $4, $5, $6, $7)
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd.ID = uuid.New()
	cmd.Status = "pending"
	cmd.CreatedAt = time.Now()

	_, err := db.Pool.Exec(ctx, query,
		cmd.ID,
		cmd.AgentID,
		cmd.Pattern,
		cmd.SearchPaths,
		cmd.Extensions,
		cmd.MaxDepth,
		cmd.Status,
	)

	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to create search command: %v", err)
		return fmt.Errorf("failed to create search command: %w", err)
	}

	db.logger.Info(logger.CategoryDatabase, "Created search command %s for agent %s", cmd.ID, cmd.AgentID)
	return nil
}

// GetFileSearchCommand retrieves a search command by ID
func (db *DB) GetFileSearchCommand(commandID uuid.UUID) (*FileSearchCommand, error) {
	query := `
        SELECT id, agent_id, pattern, search_paths, extensions, max_depth,
               status, result_count, created_at, completed_at
        FROM file_search_commands
        WHERE id = $1
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd FileSearchCommand
	err := db.Pool.QueryRow(ctx, query, commandID).Scan(
		&cmd.ID,
		&cmd.AgentID,
		&cmd.Pattern,
		&cmd.SearchPaths,
		&cmd.Extensions,
		&cmd.MaxDepth,
		&cmd.Status,
		&cmd.ResultCount,
		&cmd.CreatedAt,
		&cmd.CompletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("search command not found")
		}
		return nil, fmt.Errorf("failed to get search command: %w", err)
	}

	return &cmd, nil
}

// SaveFileSearchResults saves search results
func (db *DB) SaveFileSearchResults(searchCommandID uuid.UUID, results []FileSearchResult) error {
	if len(results) == 0 {
		return db.UpdateSearchCommandStatus(searchCommandID, "completed", 0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert results
	for _, result := range results {
		query := `
            INSERT INTO file_search_results (
                search_command_id, file_path, file_name, file_size, mod_time, permissions
            ) VALUES ($1, $2, $3, $4, $5, $6)
        `
		_, err := tx.Exec(ctx, query,
			searchCommandID,
			result.FilePath,
			result.FileName,
			result.FileSize,
			result.ModTime,
			result.Permissions,
		)
		if err != nil {
			return fmt.Errorf("failed to save search result: %w", err)
		}
	}

	// Update search command status
	updateQuery := `
        UPDATE file_search_commands
        SET status = 'completed', result_count = $1, completed_at = NOW()
        WHERE id = $2
    `
	_, err = tx.Exec(ctx, updateQuery, len(results), searchCommandID)
	if err != nil {
		return fmt.Errorf("failed to update search command: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	db.logger.Info(logger.CategoryDatabase, "Saved %d search results for command %s", len(results), searchCommandID)
	return nil
}

// GetFileSearchResults retrieves results for a search command
func (db *DB) GetFileSearchResults(searchCommandID uuid.UUID) ([]FileSearchResult, error) {
	query := `
        SELECT id, search_command_id, file_path, file_name, file_size, mod_time, permissions, created_at
        FROM file_search_results
        WHERE search_command_id = $1
        ORDER BY file_path
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Pool.Query(ctx, query, searchCommandID)
	if err != nil {
		return nil, fmt.Errorf("failed to query search results: %w", err)
	}
	defer rows.Close()

	var results []FileSearchResult
	for rows.Next() {
		var r FileSearchResult
		err := rows.Scan(
			&r.ID,
			&r.SearchCommandID,
			&r.FilePath,
			&r.FileName,
			&r.FileSize,
			&r.ModTime,
			&r.Permissions,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// UpdateSearchCommandStatus updates the status of a search command
func (db *DB) UpdateSearchCommandStatus(commandID uuid.UUID, status string, resultCount int) error {
	query := `
        UPDATE file_search_commands
        SET status = $1, result_count = $2, completed_at = NOW()
        WHERE id = $3
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Pool.Exec(ctx, query, status, resultCount, commandID)
	if err != nil {
		return fmt.Errorf("failed to update search command status: %w", err)
	}

	return nil
}

// CreateFileListCommand creates a new list command
func (db *DB) CreateFileListCommand(cmd *FileListCommand) error {
	query := `
        INSERT INTO file_list_commands (id, agent_id, path, status)
        VALUES ($1, $2, $3, $4)
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd.ID = uuid.New()
	cmd.Status = "pending"
	cmd.CreatedAt = time.Now()

	_, err := db.Pool.Exec(ctx, query, cmd.ID, cmd.AgentID, cmd.Path, cmd.Status)
	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to create list command: %v", err)
		return fmt.Errorf("failed to create list command: %w", err)
	}

	db.logger.Info(logger.CategoryDatabase, "Created list command %s for agent %s", cmd.ID, cmd.AgentID)
	return nil
}

// GetFileListCommand retrieves a list command by ID
func (db *DB) GetFileListCommand(commandID uuid.UUID) (*FileListCommand, error) {
	query := `
        SELECT id, agent_id, path, status, result_count, created_at, completed_at
        FROM file_list_commands
        WHERE id = $1
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd FileListCommand
	err := db.Pool.QueryRow(ctx, query, commandID).Scan(
		&cmd.ID,
		&cmd.AgentID,
		&cmd.Path,
		&cmd.Status,
		&cmd.ResultCount,
		&cmd.CreatedAt,
		&cmd.CompletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("list command not found")
		}
		return nil, fmt.Errorf("failed to get list command: %w", err)
	}

	return &cmd, nil
}

// SaveFileListResults saves list results
func (db *DB) SaveFileListResults(listCommandID uuid.UUID, results []FileListResult) error {
	if len(results) == 0 {
		return db.UpdateListCommandStatus(listCommandID, "completed", 0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, result := range results {
		query := `
            INSERT INTO file_list_results (
                list_command_id, file_path, file_name, file_size, mod_time, permissions, is_dir
            ) VALUES ($1, $2, $3, $4, $5, $6, $7)
        `
		_, err := tx.Exec(ctx, query,
			listCommandID,
			result.FilePath,
			result.FileName,
			result.FileSize,
			result.ModTime,
			result.Permissions,
			result.IsDir,
		)
		if err != nil {
			return fmt.Errorf("failed to save list result: %w", err)
		}
	}

	updateQuery := `
        UPDATE file_list_commands
        SET status = 'completed', result_count = $1, completed_at = NOW()
        WHERE id = $2
    `
	_, err = tx.Exec(ctx, updateQuery, len(results), listCommandID)
	if err != nil {
		return fmt.Errorf("failed to update list command: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	db.logger.Info(logger.CategoryDatabase, "Saved %d list results for command %s", len(results), listCommandID)
	return nil
}

// GetFileListResults retrieves results for a list command
func (db *DB) GetFileListResults(listCommandID uuid.UUID) ([]FileListResult, error) {
	query := `
        SELECT id, list_command_id, file_path, file_name, file_size, mod_time, permissions, is_dir, created_at
        FROM file_list_results
        WHERE list_command_id = $1
        ORDER BY file_name
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.Pool.Query(ctx, query, listCommandID)
	if err != nil {
		return nil, fmt.Errorf("failed to query list results: %w", err)
	}
	defer rows.Close()

	var results []FileListResult
	for rows.Next() {
		var r FileListResult
		err := rows.Scan(
			&r.ID,
			&r.ListCommandID,
			&r.FilePath,
			&r.FileName,
			&r.FileSize,
			&r.ModTime,
			&r.Permissions,
			&r.IsDir,
			&r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan result: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// UpdateListCommandStatus updates the status of a list command
func (db *DB) UpdateListCommandStatus(commandID uuid.UUID, status string, resultCount int) error {
	query := `
        UPDATE file_list_commands
        SET status = $1, result_count = $2, completed_at = NOW()
        WHERE id = $3
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Pool.Exec(ctx, query, status, resultCount, commandID)
	if err != nil {
		return fmt.Errorf("failed to update list command status: %w", err)
	}

	return nil
}

// CreateFileTransferCommand creates a new file transfer command
func (db *DB) CreateFileTransferCommand(cmd *FileTransferCommand) error {
	query := `
        INSERT INTO file_transfer_commands (id, agent_id, file_path, status)
        VALUES ($1, $2, $3, $4)
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd.ID = uuid.New()
	cmd.Status = "pending"
	cmd.CreatedAt = time.Now()

	_, err := db.Pool.Exec(ctx, query, cmd.ID, cmd.AgentID, cmd.FilePath, cmd.Status)
	if err != nil {
		db.logger.Error(logger.CategoryDatabase, "Failed to create transfer command: %v", err)
		return fmt.Errorf("failed to create transfer command: %w", err)
	}

	db.logger.Info(logger.CategoryDatabase, "Created transfer command %s for agent %s", cmd.ID, cmd.AgentID)
	return nil
}

// GetFileTransferCommand retrieves a transfer command by ID
func (db *DB) GetFileTransferCommand(commandID uuid.UUID) (*FileTransferCommand, error) {
	query := `
        SELECT id, agent_id, file_path, status, transferred_file_id, created_at, completed_at
        FROM file_transfer_commands
        WHERE id = $1
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd FileTransferCommand
	err := db.Pool.QueryRow(ctx, query, commandID).Scan(
		&cmd.ID,
		&cmd.AgentID,
		&cmd.FilePath,
		&cmd.Status,
		&cmd.TransferredFileID,
		&cmd.CreatedAt,
		&cmd.CompletedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("transfer command not found")
		}
		return nil, fmt.Errorf("failed to get transfer command: %w", err)
	}

	return &cmd, nil
}

// UpdateTransferCommandStatus updates the status of a transfer command
func (db *DB) UpdateTransferCommandStatus(commandID uuid.UUID, status string, fileID *uuid.UUID) error {
	query := `
        UPDATE file_transfer_commands
        SET status = $1, transferred_file_id = $2, completed_at = NOW()
        WHERE id = $3
    `

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.Pool.Exec(ctx, query, status, fileID, commandID)
	if err != nil {
		return fmt.Errorf("failed to update transfer command status: %w", err)
	}

	return nil
}

// GetPendingFileCommands gets pending file commands for an agent
func (db *DB) GetPendingFileCommands(agentID string) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var commands []interface{}
	staleAfter := 10 * time.Minute

	// Expire stale pending search commands
	_, _ = db.Pool.Exec(ctx, `
        UPDATE file_search_commands
        SET status = 'expired', completed_at = NOW()
        WHERE agent_id = $1 AND status = 'pending' AND created_at < NOW() - $2::interval
    `, agentID, fmt.Sprintf("%d minutes", int(staleAfter.Minutes())))

	// Expire stale pending list commands
	_, _ = db.Pool.Exec(ctx, `
        UPDATE file_list_commands
        SET status = 'expired', completed_at = NOW()
        WHERE agent_id = $1 AND status = 'pending' AND created_at < NOW() - $2::interval
    `, agentID, fmt.Sprintf("%d minutes", int(staleAfter.Minutes())))

	// Get pending search commands
	searchQuery := `
        SELECT id, pattern, search_paths, extensions, max_depth
        FROM file_search_commands
        WHERE agent_id = $1 AND status = 'pending'
        ORDER BY created_at
        LIMIT 10
    `

	rows, err := db.Pool.Query(ctx, searchQuery, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query search commands: %w", err)
	}

	var searchIDs []uuid.UUID
	for rows.Next() {
		var cmd FileSearchCommand
		err := rows.Scan(&cmd.ID, &cmd.Pattern, &cmd.SearchPaths, &cmd.Extensions, &cmd.MaxDepth)
		if err != nil {
			rows.Close()
			return nil, err
		}
		searchIDs = append(searchIDs, cmd.ID)
		commands = append(commands, map[string]interface{}{
			"type":         "file_search",
			"id":           cmd.ID.String(),
			"pattern":      cmd.Pattern,
			"search_paths": cmd.SearchPaths,
			"extensions":   cmd.Extensions,
			"max_depth":    cmd.MaxDepth,
		})
	}
	rows.Close()

	if len(searchIDs) > 0 {
		_, _ = db.Pool.Exec(ctx, `
            UPDATE file_search_commands
            SET status = 'in_progress'
            WHERE id = ANY($1)
        `, searchIDs)
	}

	// Get pending list commands
	listQuery := `
        SELECT id, path
        FROM file_list_commands
        WHERE agent_id = $1 AND status = 'pending'
        ORDER BY created_at
        LIMIT 10
    `

	rows, err = db.Pool.Query(ctx, listQuery, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query list commands: %w", err)
	}
	var listIDs []uuid.UUID
	for rows.Next() {
		var cmd FileListCommand
		err := rows.Scan(&cmd.ID, &cmd.Path)
		if err != nil {
			rows.Close()
			return nil, err
		}
		listIDs = append(listIDs, cmd.ID)
		commands = append(commands, map[string]interface{}{
			"type": "file_list",
			"id":   cmd.ID.String(),
			"path": cmd.Path,
		})
	}
	rows.Close()

	if len(listIDs) > 0 {
		_, _ = db.Pool.Exec(ctx, `
            UPDATE file_list_commands
            SET status = 'in_progress'
            WHERE id = ANY($1)
        `, listIDs)
	}

	// Get pending transfer commands
	transferQuery := `
        SELECT id, file_path
        FROM file_transfer_commands
        WHERE agent_id = $1 AND status = 'pending'
        ORDER BY created_at
        LIMIT 10
    `

	rows, err = db.Pool.Query(ctx, transferQuery, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query transfer commands: %w", err)
	}
	defer rows.Close()

	var transferIDs []uuid.UUID
	for rows.Next() {
		var cmd FileTransferCommand
		err := rows.Scan(&cmd.ID, &cmd.FilePath)
		if err != nil {
			return nil, err
		}
		transferIDs = append(transferIDs, cmd.ID)
		commands = append(commands, map[string]interface{}{
			"type":      "file_transfer",
			"id":        cmd.ID.String(),
			"file_path": cmd.FilePath,
		})
	}

	if len(transferIDs) > 0 {
		_, _ = db.Pool.Exec(ctx, `
            UPDATE file_transfer_commands
            SET status = 'in_progress'
            WHERE id = ANY($1)
        `, transferIDs)
	}

	return commands, nil
}
