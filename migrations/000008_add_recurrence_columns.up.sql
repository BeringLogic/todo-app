-- Add recurrence columns to todos table
ALTER TABLE todos ADD COLUMN recurrence_interval INTEGER;
ALTER TABLE todos ADD COLUMN recurrence_unit TEXT;
