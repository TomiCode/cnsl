-- kind: 'template' screens are HTML rendered by the browser; 'image' screens are just their
-- background (one image or a slideshow), processed in Go without the browser.
ALTER TABLE screens ADD COLUMN kind TEXT NOT NULL DEFAULT 'template' CHECK (kind IN ('template', 'image'));
-- layout: 'full', a framework mashup ('1Lx1R', '1Lx2R', …) or 'free' (absolutely placed regions).
ALTER TABLE screens ADD COLUMN layout TEXT NOT NULL DEFAULT 'full';
-- background: JSON {"kind":"none"|"image"|"slideshow","asset_ids":[…],"fit":…,"dither":…,"order":…}
ALTER TABLE screens ADD COLUMN background TEXT NOT NULL DEFAULT '{"kind":"none"}';
-- position in the background slideshow; advances with each stored render
ALTER TABLE screens ADD COLUMN background_index INTEGER NOT NULL DEFAULT 0;

CREATE TABLE assets (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  filename   TEXT NOT NULL UNIQUE, -- original upload in <data>/uploads, named by content hash
  mime       TEXT NOT NULL,
  width      INTEGER NOT NULL,
  height     INTEGER NOT NULL,
  size       INTEGER NOT NULL,
  created_at TEXT NOT NULL
) STRICT;

-- Static device images are replaced by image screens in playlists.
ALTER TABLE devices DROP COLUMN image_filename;
