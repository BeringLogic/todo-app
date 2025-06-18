-- Add position column to todos table to store ordering within a project
ALTER TABLE todos ADD COLUMN position INTEGER DEFAULT 0;

UPDATE todos SET position = rowid;