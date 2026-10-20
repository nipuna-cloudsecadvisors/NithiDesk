// Package assets carries the files the binaries need at run time, so a
// deployment never depends on what happens to be installed on the host.
//
// Fonts: the static (not variable) Noto Sans Sinhala and Noto Sans Latin
// faces, Regular and Bold, under the SIL Open Font License (fonts/OFL*.txt).
// Spike #26 showed that Chromium embeds static faces as ordinary TrueType
// subsets, while variable faces become Type 3 glyph drawings four times the
// size, and that system fonts (Nirmala UI, Iskoola Pota) cannot be relied on,
// least of all in a Linux container. Pin these files: any change of font
// version must be re-checked against the Sinhala shaping test strings.
package assets

import "embed"

// Fonts holds fonts/*.ttf and the licence files.
//
//go:embed fonts/*.ttf fonts/*.txt
var Fonts embed.FS
