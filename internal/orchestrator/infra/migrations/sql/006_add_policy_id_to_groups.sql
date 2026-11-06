ALTER TABLE groups ADD COLUMN policy_id INTEGER REFERENCES policies(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_groups_policy_id ON groups(policy_id);
