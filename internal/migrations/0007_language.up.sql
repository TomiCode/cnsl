-- Screen language override ("" = the global setting in settings.language).
ALTER TABLE screens ADD COLUMN language TEXT NOT NULL DEFAULT '';
