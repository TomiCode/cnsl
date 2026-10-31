CREATE TABLE connectors (
  id         INTEGER PRIMARY KEY,
  type       TEXT NOT NULL,          -- connector type key: homeassistant, openmeteo, http_json, status
  name       TEXT NOT NULL,
  config     TEXT NOT NULL DEFAULT '{}', -- JSON; may contain secrets (plaintext for now, see docs)
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

-- A binding makes a connector's data available to a screen's templates as .Data.<alias>.
-- region_key NULL = every region of the screen. Keyed by region key (not id) so bindings survive
-- layout changes.
CREATE TABLE bindings (
  id           INTEGER PRIMARY KEY,
  screen_id    INTEGER NOT NULL REFERENCES screens(id) ON DELETE CASCADE,
  region_key   TEXT,
  connector_id INTEGER NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  alias        TEXT NOT NULL,
  params       TEXT NOT NULL DEFAULT '{}',
  ttl          INTEGER NOT NULL DEFAULT 300,
  enabled      INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX bindings_screen ON bindings(screen_id);
CREATE INDEX bindings_connector ON bindings(connector_id);

-- Last fetched data per (connector, params). Kept on errors so renders can use stale data.
CREATE TABLE connector_cache (
  key           TEXT PRIMARY KEY,
  connector_id  INTEGER NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  data          TEXT,
  fetched_at    TEXT,
  expires_at    TEXT NOT NULL,
  last_error    TEXT,
  last_error_at TEXT
) STRICT;
