package plaud

import "errors"

// Validate checks that token is still accepted by the Plaud API.
//
// TODO: actually call the Plaud API. The eventual shape is a small HTTP GET
// against a known endpoint with `Authorization: Bearer <token>`; treat 401/403
// as expired/invalid and any other transport-level error as an unexpected
// failure to bubble up.
//
// Until that's wired, presence of a non-empty token is treated as valid so the
// CLI surface (audio2note plaud auth validate) is exercise-able.
func Validate(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}
