package face

import "embed"

//go:embed assets/*.svg
var embedded embed.FS

// defaultBytes returns the embedded SVG bytes for the given canonical
// expression name. Reports false if the asset is missing.
func defaultBytes(canonical string) ([]byte, bool) {
	data, err := embedded.ReadFile("assets/" + canonical + ".svg")
	if err != nil {
		return nil, false
	}
	return data, true
}
