package models

import "encoding/json"

type TranscriptModel struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	CallId  string `json:"callId"`
}

func (t *TranscriptModel) MarshalBinary() ([]byte, error) {
	return json.Marshal(t)
}

func (t *TranscriptModel) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, t)
}
