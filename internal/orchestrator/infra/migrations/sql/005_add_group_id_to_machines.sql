ALTER TABLE machines ADD COLUMN group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_machines_group_id ON machines(group_id);
