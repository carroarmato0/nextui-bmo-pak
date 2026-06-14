package clips

import "embed"

//go:embed assets/audio/*.pcm
var embeddedAssets embed.FS
