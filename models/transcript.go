package models

type TranscriptModel struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	CallId  string `json:"callId"`
}
