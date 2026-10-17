package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

var imageFolders = map[string]string{
	"home":      "link-images",
	"team":      "activity-images",
	"celebrate": "party-images",
	"when":      "category-images",
	"wiki":      "wiki-images",
}

var folders = append([]string{"photos", "pronunciation"}, slices.Collect(maps.Values(imageFolders))...)

var ImageExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

func Name(content []byte, ext string) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]) + "." + ext
}

func ImageFolder(app string) (string, bool) {
	folder, ok := imageFolders[app]
	return folder, ok
}
