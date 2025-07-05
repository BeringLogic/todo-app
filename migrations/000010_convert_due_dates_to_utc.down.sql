-- Revert due_date values from UTC back to local time (America/New_York, UTC-4)
-- Only applies to non-null due_date values
UPDATE todos SET due_date = datetime(due_date, '-4 hours') WHERE due_date IS NOT NULL;
