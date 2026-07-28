-- Migration initiale Gowlarr (Slice A).
-- Tables minimales du MVP : définitions Cardigann en cache, configuration des
-- indexeurs actifs, résultats de recherche persistés (pour que `download <id>`
-- fonctionne dans une invocation CLI séparée de `search`), cookies de session.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS indexer_definitions (
    id            TEXT NOT NULL,
    version       TEXT NOT NULL,
    yaml_sha      TEXT NOT NULL,
    downloaded_at TEXT NOT NULL,
    raw_yaml      TEXT NOT NULL,
    PRIMARY KEY (id, version)
);

CREATE TABLE IF NOT EXISTS indexer_configs (
    id          TEXT PRIMARY KEY,
    definition_id TEXT NOT NULL,
    protocol    TEXT NOT NULL, -- torrent | usenet
    enabled     INTEGER NOT NULL DEFAULT 1,
    settings_json TEXT NOT NULL DEFAULT '{}', -- identifiants/options, chiffrés au repos (post-MVP)
    proxy_url   TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cookies (
    indexer_id  TEXT PRIMARY KEY,
    cookie_json TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- Persistance des résultats de recherche : `search` et `download` sont deux
-- invocations CLI distinctes (deux process séparés), donc l'état ne peut pas
-- rester en mémoire (correction apportée suite à revue indépendante).
CREATE TABLE IF NOT EXISTS search_results (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    indexer_id    TEXT NOT NULL,
    indexer_name  TEXT NOT NULL,
    title         TEXT NOT NULL,
    details_url   TEXT,
    download_link TEXT NOT NULL,
    info_hash     TEXT,
    size_bytes    INTEGER,
    publish_date  TEXT,
    seeders       INTEGER,
    peers         INTEGER,
    grabs         INTEGER,
    categories    TEXT, -- JSON array
    protocol      TEXT NOT NULL, -- torrent | usenet
    created_at    TEXT NOT NULL,
    expires_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_search_results_expires_at ON search_results(expires_at);
