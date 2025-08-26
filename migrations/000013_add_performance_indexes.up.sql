-- Add performance indexes for better query performance

-- Index for todos table - main query optimization (project_id, completed, position)
CREATE INDEX IF NOT EXISTS idx_todos_project_completed_position ON todos (project_id, completed, position);

-- Index for todos ordering within projects
CREATE INDEX IF NOT EXISTS idx_todos_project_position ON todos (project_id, position);

-- Index for due date queries
CREATE INDEX IF NOT EXISTS idx_todos_due_date ON todos (due_date);

-- Index for ICS subscription UID matching
CREATE INDEX IF NOT EXISTS idx_todos_uid ON todos (uid);

-- Index for projects ordering
CREATE INDEX IF NOT EXISTS idx_projects_position ON projects (position);

-- Index for ICS subscriptions URL uniqueness check
CREATE INDEX IF NOT EXISTS idx_ics_subscriptions_url ON ics_subscriptions (url);

-- Index for ICS subscriptions project lookup
CREATE INDEX IF NOT EXISTS idx_ics_subscriptions_project_id ON ics_subscriptions (project_id);