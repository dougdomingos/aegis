CREATE TABLE IF NOT EXISTS propagations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	machine_id INTEGER NOT NULL,
	sequence INTEGER NOT NULL,
	kind TEXT NOT NULL,
	policy_id INTEGER,
	policy_name TEXT NOT NULL DEFAULT '',
	policy_version INTEGER NOT NULL DEFAULT 0,
	policy_type TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	sent_at DATETIME,
	acked_at DATETIME,
	nack_reason TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(machine_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_propagations_machine_sequence
	ON propagations(machine_id, sequence DESC);
