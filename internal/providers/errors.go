package providers

import "encoding/json"

func jsonDecode(r interface{ Read([]byte) (int, error) }, v any) error {
	return json.NewDecoder(r).Decode(v)
}
