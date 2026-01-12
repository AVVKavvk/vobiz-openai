package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// --- Structs for Vobiz Messages ---

type VobizInboundMessage struct {
	Event    string `json:"event"`
	StreamID string `json:"streamId"`
	Media    struct {
		Payload string `json:"payload"` // Base64 audio
	} `json:"media,omitempty"`
}

type VobizOutboundMessage struct {
	Event string      `json:"event"`
	Media *VobizMedia `json:"media,omitempty"`
}

type VobizMedia struct {
	ContentType string `json:"contentType"`
	SampleRate  int    `json:"sampleRate"`
	Payload     string `json:"payload"`
}

// --- Updated Structs for OpenAI Messages ---

type OpenAIEvent struct {
	Type       string `json:"type"`
	Audio      string `json:"audio,omitempty"` // For input_audio_buffer.append
	Delta      string `json:"delta,omitempty"` // For response.output_audio.delta
	ResponseID string `json:"response_id,omitempty"`

	// Session config (pointer so it's omitted when nil)
	Session *SessionConfig `json:"session,omitempty"`

	// Error details
	ErrorDetails *APIError `json:"error,omitempty"`
}

type SessionConfig struct {
	Modalities        []string       `json:"modalities,omitempty"`
	Instructions      string         `json:"instructions,omitempty"`
	Voice             string         `json:"voice,omitempty"`
	InputAudioFormat  string         `json:"input_audio_format,omitempty"`
	OutputAudioFormat string         `json:"output_audio_format,omitempty"`
	TurnDetection     *TurnDetection `json:"turn_detection,omitempty"`
}

type TurnDetection struct {
	Type string `json:"type"` // "server_vad"
}

type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Param   string `json:"param,omitempty"`
}

// --- Updated WebSocket Handler ---

func HandleWebSocketStream(c echo.Context) error {
	// 1. Upgrade Vobiz Connection
	vobizWs, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer vobizWs.Close()
	log.Println("✅ Vobiz Connected")

	// 2. Connect to OpenAI
	header := http.Header{}
	header.Add("Authorization", "Bearer "+OpenAIKey)
	header.Add("OpenAI-Beta", "realtime=v1")

	openAIWs, _, err := websocket.DefaultDialer.Dial(OpenAIRealtimeURL, header)
	if err != nil {
		log.Printf("❌ Failed to connect to OpenAI: %v", err)
		return err
	}
	defer openAIWs.Close()
	log.Println("✅ OpenAI Connected")

	// 3. Configure Session - Enable input transcription to get user's speech as text
	sessionUpdate := map[string]interface{}{
		"type": "session.update",
		"session": map[string]interface{}{
			"modalities":          []string{"audio", "text"},
			"instructions":        "You are Riya from Replica AI. Be concise and helpful.",
			"voice":               "alloy",
			"input_audio_format":  "g711_ulaw",
			"output_audio_format": "g711_ulaw",
			"input_audio_transcription": map[string]interface{}{
				"model": "whisper-1", // Enable transcription of user's speech
			},
			"turn_detection": map[string]interface{}{
				"type": "server_vad",
			},
		},
	}

	if err := openAIWs.WriteJSON(sessionUpdate); err != nil {
		log.Println("❌ Error sending session update:", err)
		return err
	}
	log.Println("📤 Session configuration sent")

	// Channels to handle graceful shutdown
	done := make(chan struct{})

	// --- Goroutine A: OpenAI -> Vobiz (Speaking) ---
	go func() {
		defer close(done)
		for {
			// Read raw message first to see what we're getting
			_, rawMsg, err := openAIWs.ReadMessage()
			if err != nil {
				log.Println("❌ Error reading from OpenAI:", err)
				return
			}

			// Log raw JSON for debugging
			// log.Printf("🔍 RAW OpenAI Message: %s", string(rawMsg))

			// Now parse it
			var msg map[string]interface{}
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				log.Printf("❌ Error parsing JSON: %v", err)
				continue
			}

			eventType, _ := msg["type"].(string)
			log.Printf("📩 OpenAI Event Type: %s", eventType)

			switch eventType {
			case "response.audio.delta":
				// This is the actual audio data!
				if delta, ok := msg["delta"].(string); ok && delta != "" {
					log.Printf("🔊 Got audio delta! Length: %d bytes", len(delta))
					payload := VobizOutboundMessage{
						Event: "playAudio",
						Media: &VobizMedia{
							ContentType: "audio/x-mulaw",
							SampleRate:  8000,
							Payload:     delta,
						},
					}
					if err := vobizWs.WriteJSON(payload); err != nil {
						log.Printf("❌ Error sending audio to Vobiz: %v", err)
					}
				}

			case "response.audio_transcript.delta":
				// Assistant's transcript (what AI is saying)
				if delta, ok := msg["delta"].(string); ok && delta != "" {
					log.Printf("🤖 AI says: %s", delta)
				}

			case "conversation.item.input_audio_transcription.completed":
				// USER'S TRANSCRIPT - This is what the user said!
				if transcript, ok := msg["transcript"].(string); ok && transcript != "" {
					log.Printf("👤 USER said: %s", transcript)
				}

			case "conversation.item.input_audio_transcription.delta":
				// USER'S TRANSCRIPT (streaming) - Real-time transcription
				if delta, ok := msg["delta"].(string); ok && delta != "" {
					log.Printf("👤 User speaking: %s", delta)
				}

			case "input_audio_buffer.speech_started":
				log.Println("🎤 User started talking - Clearing Vobiz buffer")
				vobizWs.WriteJSON(VobizOutboundMessage{Event: "clearAudio"})
				openAIWs.WriteJSON(map[string]string{"type": "response.cancel"})

			case "error":
				if errDetails, ok := msg["error"].(map[string]interface{}); ok {
					log.Printf("❌ OpenAI Error: %v", errDetails)
				}

			case "session.updated":
				log.Println("✅ Session Configured Successfully")

			case "response.done":
				log.Println("✅ Response completed")
				// Log the full response.done to see what's inside
				log.Printf("📋 Response Done Details: %v", msg)
			}
		}
	}()

	// --- Goroutine B: Vobiz -> OpenAI (Listening) ---
	for {
		var msg VobizInboundMessage
		err := vobizWs.ReadJSON(&msg)
		if err != nil {
			log.Println("🛑 Vobiz connection closed:", err)
			break
		}

		switch msg.Event {
		case "start":
			log.Printf("📞 Call Started (SID: %s) - Triggering Greeting...", msg.StreamID)

			// Wait a moment for session to be fully configured
			time.Sleep(200 * time.Millisecond)

			// STEP 1: Add a conversation item first
			conversationItem := map[string]interface{}{
				"type": "conversation.item.create",
				"item": map[string]interface{}{
					"type": "message",
					"role": "user",
					"content": []map[string]interface{}{
						{
							"type": "input_text",
							"text": "Hello",
						},
					},
				},
			}

			if err := openAIWs.WriteJSON(conversationItem); err != nil {
				log.Printf("❌ Error creating conversation item: %v", err)
			} else {
				log.Println("📝 Conversation item created")
			}

			// Small delay
			time.Sleep(50 * time.Millisecond)

			// STEP 2: Now create the response with explicit modalities
			triggerMsg := map[string]interface{}{
				"type": "response.create",
				"response": map[string]interface{}{
					"modalities":   []string{"audio", "text"},
					"instructions": "Introduce yourself as Riya from Replica AI and ask how you can help.",
				},
			}

			if err := openAIWs.WriteJSON(triggerMsg); err != nil {
				log.Printf("❌ Error triggering greeting: %v", err)
			} else {
				log.Println("🚀 Response creation triggered (with audio modality)")
			}

		case "media":
			if msg.Media.Payload != "" {
				openAIEvent := OpenAIEvent{
					Type:  "input_audio_buffer.append",
					Audio: msg.Media.Payload,
				}
				if err := openAIWs.WriteJSON(openAIEvent); err != nil {
					log.Printf("❌ Error sending audio to OpenAI: %v", err)
				}
			}

		case "stop":
			log.Println("🛑 Stream Stopped by Vobiz")
			return nil
		}
	}

	return nil
}
