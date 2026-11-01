-- Clockwise rotation from the designed (logical) image to the panel: 0, 90, 180 or 270.
ALTER TABLE screens ADD COLUMN rotation INTEGER NOT NULL DEFAULT 0;
