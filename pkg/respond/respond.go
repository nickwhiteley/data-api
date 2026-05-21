// Package respond provides lightweight HTTP response helpers.
package respond

import (
	"encoding/json"
	"net/http"
)

// JSON writes payload as JSON with the given status code.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
