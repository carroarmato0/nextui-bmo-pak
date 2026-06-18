// Package qr renders content into a QR-code module matrix for on-screen display.
package qr

import (
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// Matrix encodes content into a square QR-code matrix where true cells are dark
// modules. The result includes the standard quiet-zone border, so callers can
// draw it directly on a light background and have a scannable code. Medium
// error correction balances density against scan robustness on a small screen.
func Matrix(content string) ([][]bool, error) {
	if content == "" {
		return nil, fmt.Errorf("qr: empty content")
	}
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("qr: encode %q: %w", content, err)
	}
	return code.Bitmap(), nil
}
