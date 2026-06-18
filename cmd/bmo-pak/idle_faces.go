package main

import (
	"github.com/carroarmato0/nextui-bmo/internal/assistant"
	"github.com/carroarmato0/nextui-bmo/internal/face"
	"github.com/carroarmato0/nextui-bmo/internal/mod"
)

// modIdleFaces returns the set of expressions the idle scheduler may use for a
// self-contained mod — exactly the faces it ships on disk — so idle cycles the
// mod's real faces instead of folding unshipped expressions to neutral (which
// looks static on screen). It returns nil for the default/overlay mod, which
// resolves every canonical face via the embedded set, leaving idle unfiltered.
func modIdleFaces(m mod.Mod) map[assistant.Expression]bool {
	if !m.SelfContained() {
		return nil
	}
	names := face.FaceNamesInDir(m.FacesDir())
	if len(names) == 0 {
		return nil
	}
	set := make(map[assistant.Expression]bool, len(names))
	for _, n := range names {
		set[assistant.Expression(n)] = true
	}
	return set
}
