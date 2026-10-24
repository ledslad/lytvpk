package app

import (
	"path/filepath"
	"strings"
)

// Workshop downloads use the actual downloadable item's ID, rather than a
// collection ID or title. Preserve non-VPK extensions so archives are never
// disguised as VPKs; missing extensions default to the game's VPK format.
func workshopDownloadFilename(workshopID, sourceFilename string) string {
	filename := cleanFilename(sourceFilename)
	if strings.HasPrefix(workshopID, "direct-") {
		return filename
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || ext == "." {
		ext = ".vpk"
	}
	return workshopID + ext
}
