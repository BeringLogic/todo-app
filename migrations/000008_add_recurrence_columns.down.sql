-- Remove recurrence columns from todos table
ALTER TABLE todos DROP COLUMN recurrence_interval;
ALTER TABLE todos DROP COLUMN recurrence_unit;
