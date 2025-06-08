-- Users Table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    telegram_id BIGINT UNIQUE NOT NULL,
    username TEXT UNIQUE,
    first_name TEXT,
    last_name TEXT,
    is_admin BOOLEAN DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);
CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);

-- Incidents Table
CREATE TABLE IF NOT EXISTS incidents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    fingerprint TEXT UNIQUE NOT NULL,
    status TEXT NOT NULL,
    starts_at DATETIME,
    ends_at DATETIME,
    summary TEXT,
    description TEXT,
    labels TEXT,
    affected_resources TEXT,
    resolved_by INTEGER,
    rejection_reason TEXT,
    FOREIGN KEY (resolved_by) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_incidents_deleted_at ON incidents(deleted_at);
CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);
CREATE INDEX IF NOT EXISTS idx_incidents_fingerprint ON incidents(fingerprint);

-- AuditRecords Table
CREATE TABLE IF NOT EXISTS audit_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    incident_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    action TEXT,
    parameters TEXT,
    timestamp DATETIME NOT NULL,
    success BOOLEAN,
    result TEXT,
    FOREIGN KEY (incident_id) REFERENCES incidents(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_audit_records_deleted_at ON audit_records(deleted_at);
CREATE INDEX IF NOT EXISTS idx_audit_records_incident_id ON audit_records(incident_id);
