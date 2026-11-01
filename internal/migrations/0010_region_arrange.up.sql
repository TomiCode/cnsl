-- How a region arranges its content (JSON, see render.Arrange; '' = the template brings its own
-- layout) and its blocks (JSON, see render.Block; '' = the template is edited as HTML).
ALTER TABLE regions ADD COLUMN arrange TEXT NOT NULL DEFAULT '';
ALTER TABLE regions ADD COLUMN blocks TEXT NOT NULL DEFAULT '';
