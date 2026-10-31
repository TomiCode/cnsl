-- How a model maps onto the framework's CSS: the page is laid out at width/scale × height/scale
-- CSS pixels and screenshotted at device-pixel-ratio = scale.
ALTER TABLE device_models ADD COLUMN scale REAL NOT NULL DEFAULT 1;
ALTER TABLE device_models ADD COLUMN screen_classes TEXT NOT NULL DEFAULT 'screen';

UPDATE device_models SET scale = 1.8, screen_classes = 'screen screen--v2 screen--lg screen--density-2x screen--4bit'
  WHERE key = 'trmnl_x';
UPDATE device_models SET scale = 1, screen_classes = 'screen screen--og screen--sm screen--4bit'
  WHERE key = 'virtual';

CREATE TABLE screens (
  id                INTEGER PRIMARY KEY,
  name              TEXT NOT NULL,
  model_id          INTEGER NOT NULL REFERENCES device_models(id),
  framework_version TEXT NOT NULL,
  head              TEXT NOT NULL DEFAULT '',   -- extra <head> HTML/CSS
  render_interval   INTEGER NOT NULL DEFAULT 900, -- seconds between scheduled renders
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
) STRICT;

-- Phase 2 screens have one full-size region ('main'); layouts add more in phase 3.
CREATE TABLE regions (
  id          INTEGER PRIMARY KEY,
  screen_id   INTEGER NOT NULL REFERENCES screens(id) ON DELETE CASCADE,
  key         TEXT NOT NULL,
  x INTEGER NOT NULL, y INTEGER NOT NULL, w INTEGER NOT NULL, h INTEGER NOT NULL,
  z           INTEGER NOT NULL DEFAULT 0,
  template    TEXT NOT NULL DEFAULT '',
  class       TEXT NOT NULL DEFAULT '',
  transparent INTEGER NOT NULL DEFAULT 0,
  UNIQUE (screen_id, key)
) STRICT;

CREATE TABLE renders (
  id          INTEGER PRIMARY KEY,
  screen_id   INTEGER NOT NULL REFERENCES screens(id) ON DELETE CASCADE,
  filename    TEXT,                           -- content hash; NULL when the render failed
  status      TEXT NOT NULL CHECK (status IN ('ok', 'error')),
  error       TEXT,
  duration_ms INTEGER NOT NULL,
  trigger     TEXT NOT NULL,
  created_at  TEXT NOT NULL
) STRICT;
CREATE INDEX renders_screen ON renders(screen_id, id);

CREATE TABLE playlists (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE playlist_items (
  id           INTEGER PRIMARY KEY,
  playlist_id  INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
  screen_id    INTEGER NOT NULL REFERENCES screens(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  dwell        INTEGER NOT NULL DEFAULT 1 CHECK (dwell >= 1),
  refresh_rate INTEGER,                       -- NULL = the device's own rate
  enabled      INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX playlist_items_playlist ON playlist_items(playlist_id, position);

ALTER TABLE devices ADD COLUMN playlist_id INTEGER REFERENCES playlists(id) ON DELETE SET NULL;
ALTER TABLE devices ADD COLUMN calibrating_until TEXT;

-- Where each device is in its playlist rotation.
CREATE TABLE device_playback (
  device_id   INTEGER PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
  item_id     INTEGER REFERENCES playlist_items(id) ON DELETE SET NULL,
  shown_count INTEGER NOT NULL DEFAULT 0
) STRICT;
