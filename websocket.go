package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// --- Structs for Vobiz Messages ---

type VobizInboundMessage struct {
	Event    string `json:"event"`
	StreamID string `json:"streamId"` // Present in 'media' events
	Start    struct {
		CallId    string `json:"callId"`
		StreamId  string `json:"streamId"` // Present in 'start' events
		AccountId string `json:"accountId"`
	} `json:"start,omitempty"`
	Media struct {
		Payload string `json:"payload"`
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
	var OpenAIKey = os.Getenv("OPENAI_API_KEY")
	var callId string
	var transcripts []map[string]interface{}
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
			"instructions":        "You are Anika, a claims support agent at KIWI Insurance. You are empathetic, efficient, and reassuring. Your goal is to guide customers through the First Notice of Loss (FNOL) process for Motor insurance. CRITICAL: Follow a Zero-Repetition Policy. NEVER repeat or paraphrase any information provided by the customer (e.g., locations, registration numbers, or incident details). Use phrases like 'I have noted that' or 'That is recorded' and move immediately to the next question. Follow these steps: 1. Ensure Safety First: Confirm the customer and others are safe. 2. Build Calm Reassurance: Briefly acknowledge the situation empathetically. 3. Collect FNOL Info: Ask for Vehicle Registration Number (starts with state codes like MH, KA, DL). 4. Relationship: Confirm if they are the policyholder. 5. Open Narration: Ask 'Can you briefly tell me what happened?' and extract What, Where, When, Damage, and Third-party details. 6. Gap Filling: Ask only for missing details. 7. Police/FIR: Ask only if there are injuries or third-party damage. 8. Closing: Confirm current location, provide Claim Reference Number #123098, and send the WhatsApp link for photos. Ask only one short question at a time. Do not offer legal/medical advice or speculate on claim outcomes.
			If user done with call or ask for end call then use call_end function.
			if user asked or if user information needed for their info then use get_customer_info function.",
			"voice":               "alloy",
			"input_audio_format":  "g711_ulaw",
			"output_audio_format": "g711_ulaw",
			"input_audio_transcription": map[string]interface{}{
				"model": "whisper-1",
			},
			"tools": []map[string]interface{}{
				{
					"type":        "function",
					"name":        "call_end",
					"description": "Ends the current phone call immediately.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"callId": map[string]interface{}{"type": "string"},
						},
						"required": []string{"callId"},
					},
				},
				{
					"type":        "function",
					"name":        "get_customer_info",
					"description": "Retrieves the user's name, age, and address.",
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
			},
			"tool_choice": "auto",
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

			// fmt.Println("Openai MGS: %v", msg)

			eventType, _ := msg["type"].(string)
			log.Printf("--->>>>📩 ---->>> OpenAI Event Type: %s", eventType)

			switch eventType {
			case "response.audio.delta":
				// This is the actual audio data!
				if delta, ok := msg["delta"].(string); ok && delta != "" {
					// log.Printf("🔊 Got audio delta! Length: %d bytes", len(delta))
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

			// case "response.audio_transcript.delta":
			// 	// Assistant's transcript (what AI is saying)
			// 	if delta, ok := msg["delta"].(string); ok && delta != "" {
			// 		log.Printf("🤖 AI says: %s", delta)
			// 	}
			case "response.audio_transcript.done":
				// Assistant's transcript (what AI is saying)
				fmt.Println("Openai MGS: %v", msg)
				if delta, ok := msg["transcript"].(string); ok && delta != "" {
					log.Printf("🤖 AI whole transcript: %s", delta)
					transcript := map[string]interface{}{
						"role":    "assistant",
						"content": delta,
					}
					transcripts = append(transcripts, transcript)
				}

			case "conversation.item.input_audio_transcription.completed":
				// USER'S TRANSCRIPT - This is what the user said!
				fmt.Println("Openai MGS: %v", msg)
				if transcript, ok := msg["transcript"].(string); ok && transcript != "" {
					log.Printf("👤 USER said: %s", transcript)
					trans := map[string]interface{}{
						"role":    "assistant",
						"content": transcript,
					}
					transcripts = append(transcripts, trans)
				}

			// case "conversation.item.input_audio_transcription.delta":
			// 	// USER'S TRANSCRIPT (streaming) - Real-time transcription
			// 	if delta, ok := msg["delta"].(string); ok && delta != "" {
			// 		log.Printf("👤 User speaking: %s", delta)
			// 	}

			case "input_audio_buffer.speech_started":
				fmt.Println("Openai MGS: %v", msg)
				log.Println("🎤 User started talking - Clearing Vobiz buffer")
				vobizWs.WriteJSON(VobizOutboundMessage{Event: "clearAudio"})
				openAIWs.WriteJSON(map[string]string{"type": "response.cancel"})

			case "error":
				fmt.Println("Openai MGS: %v", msg)
				if errDetails, ok := msg["error"].(map[string]interface{}); ok {
					log.Printf("❌ OpenAI Error: %v", errDetails)
				}

			case "session.updated":
				fmt.Println("Openai MGS: %v", msg)
				log.Println("✅ Session Configured Successfully")

			case "response.done":
				fmt.Println("Openai MGS: %v", msg)
				log.Println("✅ Response completed")
				// Log the full response.done to see what's inside
				log.Printf("📋 Response Done Details: %v", msg)
			case "response.function_call_arguments.done":
				fmt.Println("Openai MGS: %v", msg)
				// OpenAI has finished generating arguments for a function
				fnName, _ := msg["name"].(string)
				argsRaw, _ := msg["arguments"].(string)
				callID, _ := msg["call_id"].(string) // OpenAI's internal tool call ID

				log.Printf("🛠️ Tool Call: %s with args: %s", fnName, argsRaw)

				var toolOutput interface{}

				if fnName == "get_customer_info" {
					toolOutput = getCustomerInfo()
				} else if fnName == "call_end" {
					// Parse the callId from the AI's arguments
					var args struct {
						CallId string `json:"callId"`
					}
					json.Unmarshal([]byte(argsRaw), &args)

					// Use the callId provided by the AI, or fallback to the one captured in 'start' event
					targetID := args.CallId
					if targetID == "" {
						targetID = callId
					}

					err := callEnd(callId)
					if err != nil {
						toolOutput = map[string]string{"error": err.Error()}
					} else {
						toolOutput = map[string]string{"status": "call_terminated"}
					}
				}

				// STEP 3: Send the result back to OpenAI
				outputBytes, _ := json.Marshal(toolOutput)
				responseEvent := map[string]interface{}{
					"type": "conversation.item.create",
					"item": map[string]interface{}{
						"type":    "function_call_output",
						"call_id": callID,
						"output":  string(outputBytes),
					},
				}
				openAIWs.WriteJSON(responseEvent)

				// Trigger the AI to acknowledge the info and continue speaking
				openAIWs.WriteJSON(map[string]interface{}{"type": "response.create"})
			}

		}
	}()

	// --- Goroutine B: Vobiz -> OpenAI (Listening) ---
	for {

		var msg VobizInboundMessage
		err = vobizWs.ReadJSON(&msg)
		if err != nil {
			log.Println("🛑 Vobiz connection closed:", err)
			break
		}

		switch msg.Event {
		case "start":
			callId = msg.Start.CallId

			// %+v is your best friend for debugging structs!
			log.Printf("Full Message Received: %+v", msg)
			log.Printf("📞 Call Started (SID: %s) (CallID: %s)", msg.Start.StreamId, msg.Start.CallId)
			log.Println(callId)

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
					"instructions": "Introduce yourself as Hello, I'm Anika from KIWI Insurance and ask how you can help.",
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
			jsonData, err := json.MarshalIndent(transcripts, "", "    ")
			if err != nil {
				log.Fatalf("Error encoding JSON: %s", err)
			}

			// Write to transcript.json
			err = os.WriteFile("transcript.json", jsonData, 0644)
			if err != nil {
				log.Fatalf("Error writing file: %s", err)
			}

			log.Println("Successfully saved to transcript.json")
			return nil
		}
	}

	return nil
}

func callEnd(callId string) error {
	var VobizAuthID = os.Getenv("VOBIZ_AUTH_ID")
	var VobizAuthToken = os.Getenv("VOBIZ_AUTH_TOKEN")

	// Construct the URL using the account ID and call UUID
	url := fmt.Sprintf("https://api.vobiz.ai/api/v1/Account/%s/Call/%s/", VobizAuthID, callId)

	// Create a new DELETE request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add the required headers
	req.Header.Set("X-Auth-ID", VobizAuthID)
	req.Header.Set("X-Auth-Token", VobizAuthToken)
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response status is successful (usually 200 OK or 204 No Content)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vobiz api returned error status: %s", resp.Status)
	}

	log.Printf("Successfully terminated call: %s", callId)
	return nil
}

func getCustomerInfo() map[string]interface{} {

	return map[string]interface{}{
		"name":    "Vipin Kumawat",
		"age":     22,
		"gender":  "Male",
		"address": "Village Bhoya, Post Harsh , Sikar, Rajasthan , India, 332021",
	}
}
