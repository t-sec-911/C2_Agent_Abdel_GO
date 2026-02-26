package database

import (
	"context"
	"fmt"
	"sOPown3d/server/logger"
)

// Related to transfer files and saving files
const createTransferredFilesTable = `
CREATE TABLE IF NOT EXISTS transferred_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(255) NOT NULL,
    original_path TEXT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    stored_path TEXT NOT NULL,
    size BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    mod_time TIMESTAMP NOT NULL,
    permissions VARCHAR(20),
    uploaded_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_files ON transferred_files(agent_id);
CREATE INDEX IF NOT EXISTS idx_filename ON transferred_files(filename);
CREATE INDEX IF NOT EXISTS idx_uploaded_at ON transferred_files(uploaded_at);
CREATE INDEX IF NOT EXISTS idx_checksum ON transferred_files(checksum);
`
const createAgentsTable = `
CREATE TABLE IF NOT EXISTS agents (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(255) UNIQUE NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    os VARCHAR(50) NOT NULL,
    username VARCHAR(255),
    first_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    is_active BOOLEAN DEFAULT true,
    inactive_threshold_minutes INTEGER DEFAULT 5,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agents_agent_id ON agents(agent_id);
CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents(last_seen);
CREATE INDEX IF NOT EXISTS idx_agents_is_active ON agents(is_active);
`

const createExecutionsTable = `
CREATE TABLE IF NOT EXISTS command_executions (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(255) NOT NULL,
    command_action VARCHAR(100) NOT NULL,
    command_payload TEXT,
    output TEXT,
    executed_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_executions_agent_id ON command_executions(agent_id);
CREATE INDEX IF NOT EXISTS idx_executions_executed_at ON command_executions(executed_at);
CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions(created_at);
`

const createFileSearchCommandsTable = `
CREATE TABLE IF NOT EXISTS file_search_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(255) NOT NULL,
    pattern VARCHAR(255),
    search_paths TEXT[],
    extensions TEXT[],
    max_depth INT DEFAULT 5,
    status VARCHAR(20) DEFAULT 'pending',
    result_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_fsc_agent ON file_search_commands(agent_id);
CREATE INDEX IF NOT EXISTS idx_fsc_status ON file_search_commands(status);
`

const createFileSearchResultsTable = `
CREATE TABLE IF NOT EXISTS file_search_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    search_command_id UUID NOT NULL REFERENCES file_search_commands(id),
    file_path TEXT NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT,
    mod_time TIMESTAMP,
    permissions VARCHAR(20),
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fsr_command ON file_search_results(search_command_id);
`

const createFileTransferCommandsTable = `
CREATE TABLE IF NOT EXISTS file_transfer_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(255) NOT NULL,
    file_path TEXT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    transferred_file_id UUID REFERENCES transferred_files(id),
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ftc_agent ON file_transfer_commands(agent_id);
CREATE INDEX IF NOT EXISTS idx_ftc_status ON file_transfer_commands(status);
`

const createFileListCommandsTable = `
CREATE TABLE IF NOT EXISTS file_list_commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id VARCHAR(255) NOT NULL,
    path TEXT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    result_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_flc_agent ON file_list_commands(agent_id);
CREATE INDEX IF NOT EXISTS idx_flc_status ON file_list_commands(status);
`

const createFileListResultsTable = `
CREATE TABLE IF NOT EXISTS file_list_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    list_command_id UUID NOT NULL REFERENCES file_list_commands(id),
    file_path TEXT NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT DEFAULT 0,
    mod_time TIMESTAMP,
    permissions VARCHAR(20),
    is_dir BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_flr_command ON file_list_results(list_command_id);
`

// RunMigrations executes all database migrations

func (db *DB) RunMigrations(ctx context.Context) error {
	db.logger.Info(logger.CategoryDatabase, "Running database migrations...")

	migrations := []struct {
		name  string
		query string
	}{
		{"agents", createAgentsTable},
		{"command_executions", createExecutionsTable},
		{"transferred_files", createTransferredFilesTable},
		{"file_search_commands", createFileSearchCommandsTable},
		{"file_search_results", createFileSearchResultsTable},
		{"file_transfer_commands", createFileTransferCommandsTable},
		{"file_list_commands", createFileListCommandsTable},
		{"file_list_results", createFileListResultsTable},
	}

	for _, m := range migrations {
		if _, err := db.Pool.Exec(ctx, m.query); err != nil {
			return fmt.Errorf("failed to create %s table: %w", m.name, err)
		}
		db.logger.Info(logger.CategorySuccess, "Table '%s' created/verified", m.name)
	}

	db.logger.Info(logger.CategorySuccess, "All migrations complete")
	return nil
}
