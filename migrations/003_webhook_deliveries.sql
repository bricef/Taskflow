-- Webhook delivery log: records every delivery attempt.
CREATE TABLE webhook_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    webhook_id INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_id TEXT NOT NULL,          -- event timestamp as unique-ish identifier
    attempt INTEGER NOT NULL DEFAULT 1,
    status_code INTEGER,             -- NULL if request failed before response
    error TEXT,                      -- NULL on success
    request_body TEXT NOT NULL,      -- the JSON payload sent
    duration_ms INTEGER,             -- delivery round-trip time in milliseconds
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX idx_webhook_deliveries_created_at ON webhook_deliveries(created_at);
