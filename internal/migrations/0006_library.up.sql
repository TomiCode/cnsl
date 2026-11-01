-- Reusable template parts, called from region templates as {{ component "key" … }}.
-- Built-ins ship in the binary and are refreshed on every start (builtin = 1, read-only).
CREATE TABLE components (
  id             INTEGER PRIMARY KEY,
  key            TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL,
  description    TEXT NOT NULL DEFAULT '',
  source         TEXT NOT NULL,
  inputs         TEXT NOT NULL DEFAULT '[]', -- JSON: [{name, label, kind, connector, default, help}]
  builtin        INTEGER NOT NULL DEFAULT 0,
  updated_at     TEXT NOT NULL
) STRICT;

-- Editor text snippets (Monaco snippet syntax).
CREATE TABLE snippets (
  id          INTEGER PRIMARY KEY,
  prefix      TEXT NOT NULL UNIQUE,  -- what you type to get it
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL,
  builtin     INTEGER NOT NULL DEFAULT 0,
  updated_at  TEXT NOT NULL
) STRICT;
