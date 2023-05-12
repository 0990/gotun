package crypto

import "errors"

type Mode int

const (
	None Mode = iota
	GCM
)

func ToMode(s string) (Mode, error) {
	switch s {
	case "", "none", "NONE":
		return None, nil
	case "gcm", "GCM":
		return GCM, nil
	default:
		return -1, errors.New("unknown mode")
	}
}
