-- Remove performance indexes

DROP INDEX IF EXISTS idx_todos_project_completed_position;
DROP INDEX IF EXISTS idx_todos_project_position;
DROP INDEX IF EXISTS idx_todos_due_date;
DROP INDEX IF EXISTS idx_todos_uid;
DROP INDEX IF EXISTS idx_projects_position;
DROP INDEX IF EXISTS idx_ics_subscriptions_url;
DROP INDEX IF EXISTS idx_ics_subscriptions_project_id;