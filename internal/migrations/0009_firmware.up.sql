-- Firmware images for over-the-air updates (files in <data>/firmware/<sha256>.bin).
CREATE TABLE firmware (
  id         INTEGER PRIMARY KEY,
  version    TEXT NOT NULL,              -- what devices report in FW-Version after installing it
  chip       TEXT NOT NULL,              -- esp32, esp32s3, … from the image header
  project    TEXT NOT NULL DEFAULT '',
  built      TEXT NOT NULL DEFAULT '',
  idf        TEXT NOT NULL DEFAULT '',
  size       INTEGER NOT NULL,
  sha256     TEXT NOT NULL UNIQUE,
  filename   TEXT NOT NULL,              -- name of the uploaded file
  notes      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
) STRICT;

-- The chip a model's firmware must be built for, and the firmware its devices should run.
-- References are kept by the application (no FOREIGN KEY, so the columns can be dropped again).
ALTER TABLE device_models ADD COLUMN chip TEXT NOT NULL DEFAULT '';
ALTER TABLE device_models ADD COLUMN firmware_id INTEGER;
UPDATE device_models SET chip = 'esp32s3' WHERE key = 'trmnl_x';

-- Opt-in per device, plus the state of the current offer.
ALTER TABLE devices ADD COLUMN auto_update INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN ota_firmware_id INTEGER;  -- firmware last offered
ALTER TABLE devices ADD COLUMN ota_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN ota_offered_at TEXT;
