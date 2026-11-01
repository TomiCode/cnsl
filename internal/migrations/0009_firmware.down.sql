ALTER TABLE devices DROP COLUMN ota_offered_at;
ALTER TABLE devices DROP COLUMN ota_attempts;
ALTER TABLE devices DROP COLUMN ota_firmware_id;
ALTER TABLE devices DROP COLUMN auto_update;
ALTER TABLE device_models DROP COLUMN firmware_id;
ALTER TABLE device_models DROP COLUMN chip;
DROP TABLE firmware;
