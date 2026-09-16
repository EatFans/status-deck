package protocol

import (
	"encoding/json"
	"time"

	"status-deck/desktop/internal/collectors"
)

type StatusPayload struct {
	Type      string                     `json:"type"`
	Version   int                        `json:"version"`
	Timestamp time.Time                  `json:"timestamp"`
	System    collectors.SystemSnapshot  `json:"system"`
}

func NewStatusPayload(system collectors.SystemSnapshot) StatusPayload {
	return StatusPayload{
		Type:      "status",
		Version:   1,
		Timestamp: time.Now(),
		System:    system,
	}
}

func Encode(payload StatusPayload) ([]byte, error) {
	return json.Marshal(payload)
}
