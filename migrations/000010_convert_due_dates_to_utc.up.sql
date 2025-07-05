-- Convert due_date values from local time (America/New_York, UTC-4) to UTC
-- Only applies to non-null due_date values
UPDATE todos SET due_date = datetime(due_date, '+4 hours') WHERE due_date IS NOT NULL;
