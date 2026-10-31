CREATE TABLE device_models (
  id             INTEGER PRIMARY KEY,
  key            TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL,
  firmware_model TEXT,                    -- value of the firmware's Model header
  width          INTEGER NOT NULL,
  height         INTEGER NOT NULL,
  bit_depth      INTEGER NOT NULL CHECK (bit_depth IN (1, 2, 4)),
  rotation       INTEGER NOT NULL DEFAULT 0,
  tone_curve     TEXT,
  builtin        INTEGER NOT NULL DEFAULT 0
) STRICT;

INSERT INTO device_models (key, name, firmware_model, width, height, bit_depth, builtin) VALUES
  ('trmnl_x', 'TRMNL X', 'x', 1872, 1404, 4, 1),
  ('virtual', 'Virtual device', 'virtual', 800, 480, 4, 1);

CREATE TABLE devices (
  id                INTEGER PRIMARY KEY,
  mac               TEXT NOT NULL UNIQUE,
  api_key           TEXT NOT NULL UNIQUE,
  friendly_id       TEXT NOT NULL UNIQUE,
  label             TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL CHECK (status IN ('pending', 'active', 'disabled')),
  model_id          INTEGER REFERENCES device_models(id),
  refresh_rate      INTEGER NOT NULL DEFAULT 900,
  sleep_start       TEXT,
  sleep_end         TEXT,
  image_filename    TEXT,                 -- static image until screens exist (phase 2)
  firmware_version  TEXT,
  reported_model    TEXT,
  reported_width    INTEGER,
  reported_height   INTEGER,
  last_seen_at      TEXT,
  approved_at       TEXT,
  created_at        TEXT NOT NULL,
  updated_at        TEXT NOT NULL
) STRICT;

CREATE TABLE device_telemetry (
  id               INTEGER PRIMARY KEY,
  device_id        INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  at               TEXT NOT NULL,
  battery_voltage  REAL,
  battery_percent  REAL,
  charging         INTEGER,
  usb_connected    INTEGER,
  rssi             INTEGER,
  wake_reason      TEXT,
  wake_time        INTEGER,
  firmware_version TEXT,
  image_cached     INTEGER,
  served_filename  TEXT,
  raw_headers      TEXT
) STRICT;
CREATE INDEX device_telemetry_device ON device_telemetry(device_id, id);

CREATE TABLE device_logs (
  id            INTEGER PRIMARY KEY,
  device_id     INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  device_log_id INTEGER,
  level         TEXT,
  message       TEXT NOT NULL,
  source_path   TEXT,
  source_line   INTEGER,
  device_time   TEXT,
  received_at   TEXT NOT NULL,
  extra         TEXT
) STRICT;
CREATE INDEX device_logs_device ON device_logs(device_id, id);
