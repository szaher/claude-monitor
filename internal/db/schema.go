package db

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_path TEXT NOT NULL,
	project_name TEXT NOT NULL,
	cwd TEXT,
	git_branch TEXT,
	started_at DATETIME NOT NULL,
	ended_at DATETIME,
	claude_version TEXT,
	entry_point TEXT,
	permission_mode TEXT,
	total_input_tokens INTEGER DEFAULT 0,
	total_output_tokens INTEGER DEFAULT 0,
	total_cache_read_tokens INTEGER DEFAULT 0,
	total_cache_write_tokens INTEGER DEFAULT 0,
	estimated_cost_usd REAL DEFAULT 0.0
);

CREATE INDEX IF NOT EXISTS idx_sessions_project_started ON sessions(project_path, started_at);

CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	parent_id TEXT,
	type TEXT NOT NULL,
	role TEXT,
	model TEXT,
	content_text TEXT,
	content_json TEXT,
	stop_reason TEXT,
	input_tokens INTEGER DEFAULT 0,
	output_tokens INTEGER DEFAULT 0,
	cache_read_tokens INTEGER DEFAULT 0,
	cache_write_tokens INTEGER DEFAULT 0,
	timestamp DATETIME NOT NULL,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_messages_session_timestamp ON messages(session_id, timestamp);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(id, content_text, content='messages', content_rowid='rowid');

CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
	INSERT INTO messages_fts(rowid, id, content_text) VALUES (new.rowid, new.id, new.content_text);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
	DELETE FROM messages_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
	DELETE FROM messages_fts WHERE rowid = old.rowid;
	INSERT INTO messages_fts(rowid, id, content_text) VALUES (new.rowid, new.id, new.content_text);
END;

CREATE TABLE IF NOT EXISTS tool_calls (
	id TEXT PRIMARY KEY,
	message_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	tool_input TEXT,
	tool_response TEXT,
	success BOOLEAN DEFAULT 1,
	error TEXT,
	duration_ms INTEGER,
	timestamp DATETIME NOT NULL,
	FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session_tool ON tool_calls(session_id, tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_calls_tool_timestamp ON tool_calls(tool_name, timestamp);

CREATE TABLE IF NOT EXISTS subagents (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	agent_type TEXT NOT NULL,
	description TEXT,
	started_at DATETIME NOT NULL,
	ended_at DATETIME,
	FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_subagents_session ON subagents(session_id);
`
