CREATE TABLE ics_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    project_id INTEGER NOT NULL,
    last_updated_at TIMESTAMP,
    FOREIGN KEY(project_id) REFERENCES projects(id)
);