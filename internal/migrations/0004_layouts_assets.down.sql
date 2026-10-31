ALTER TABLE devices ADD COLUMN image_filename TEXT;
DROP TABLE assets;
ALTER TABLE screens DROP COLUMN background_index;
ALTER TABLE screens DROP COLUMN background;
ALTER TABLE screens DROP COLUMN layout;
ALTER TABLE screens DROP COLUMN kind;
