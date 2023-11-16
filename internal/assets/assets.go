package assets

import "embed"

//go:embed index.html ui.html app css js
var Assets embed.FS
