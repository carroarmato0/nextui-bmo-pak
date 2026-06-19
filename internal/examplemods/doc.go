// Package examplemods holds validation tests for the example mods shipped under
// examples/mods, run through the same libraries the app uses. It has no runtime
// code — it exists so `go test` guards the example mods against silent rot,
// without placing Go files inside the data-only showcase directories themselves.
package examplemods
