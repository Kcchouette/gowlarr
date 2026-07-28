CREATE TABLE IF NOT EXISTS encryption_keys (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    salt       BLOB NOT NULL,
    created_at TEXT NOT NULL
);

ALTER TABLE indexer_configs ADD COLUMN settings_enc BLOB;
ALTER TABLE cookies ADD COLUMN cookie_enc BLOB;
