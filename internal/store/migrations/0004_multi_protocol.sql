-- Migration 0004 : protocoles ddl/streaming (affichage seul).
-- Trois ALTER TABLE séparés (SQLite : un par colonne).

ALTER TABLE search_results ADD COLUMN hosts TEXT;
ALTER TABLE search_results ADD COLUMN unlocked INTEGER NOT NULL DEFAULT 1;
ALTER TABLE search_results ADD COLUMN stream_url TEXT;
