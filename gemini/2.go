package gemini

// ALTERNATIVE VERSION: Uses Google AI API with Gemini 2.0 Flash Live
// This version works with API keys instead of requiring Vertex AI setup

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/AVVKavvk/openai-vobiz/models"
	"github.com/AVVKavvk/openai-vobiz/rabbitmq"
	"github.com/AVVKavvk/openai-vobiz/redisClient"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// Note: Structs are the same as in the main file, so they're omitted here
// to avoid duplication. In practice, you'd use one file or the other.

func HandleWebSocketStreamGoogleAI(c echo.Context) error {
	var GeminiAPIKey = os.Getenv("GEMINI_API_KEY") // Use API Key instead of access token
	var callId string

	// Get the parameters from the URL
	from := c.QueryParam("from")
	to := c.QueryParam("to")
	uuid := c.QueryParam("calluuid")

	log.Printf("WS Connection for Call %s: From %s to %s", uuid, from, to)

	// 1. Upgrade Vobiz Connection
	vobizWs, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer vobizWs.Close()
	log.Println("✅ Vobiz Connected")

	// 2. Connect to Gemini Live API via Google AI
	// Google AI API uses API keys and supports Gemini 2.0 models
	geminiURL := fmt.Sprintf(
		"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
		GeminiAPIKey,
	)

	header := http.Header{}
	// Google AI API doesn't need Authorization header - it uses the key in URL

	geminiWs, _, err := websocket.DefaultDialer.Dial(geminiURL, header)
	if err != nil {
		log.Printf("❌ Failed to connect to Gemini: %v", err)
		return err
	}
	defer geminiWs.Close()
	log.Println("✅ Gemini Connected via Google AI API")

	// 3. Configure Session with Gemini 2.0 Flash Live
	// Note: Google AI API currently supports 2.0 models, not 2.5
	setupMsg := GeminiClientMessage{
		Setup: &GeminiSetup{
			Model: "models/gemini-2.5-flash-native-audio-preview-12-2025", // Use 2.0 model for Google AI API
			GenerationConfig: &GeminiGenerationConfig{
				ResponseModalities: []string{"AUDIO"},
				SpeechConfig: &GeminiSpeechConfig{
					VoiceConfig: &GeminiVoiceConfig{
						PrebuiltVoiceConfig: &GeminiPrebuiltVoiceConfig{
							VoiceName: "Puck", // Available voices: Puck, Charon, Kore, Fenrir, Aoede
						},
					},
				},
				Temperature: 0.8,
				TopP:        0.95,
			},
			SystemInstruction: &GeminiContent{
				Parts: []GeminiPart{
					{
						Text: `You are Anika, a claims support agent at KIWI Insurance. You are empathetic, efficient, and reassuring. 

### CORE POLICIES:
1. ZERO-REPETITION: Never repeat customer details. Use "Recorded" or "I have that noted" and move on.
2. ONE QUESTION AT A TIME: Keep responses short and focused.
3. SAFETY FIRST: Always confirm safety before data collection.

### FUNCTION CALLING PROTOCOLS:
- **get_customer_info**: Call this immediately if the user asks "What information do you have on me?" or if you need to verify their identity/address to proceed with the claim. Do not guess their details; use the tool.
- **call_end**: Trigger this tool ONLY when:
    a) The customer says goodbye or indicates they want to hang up.
    b) You have provided the Claim Reference Number (#123098) and confirmed the WhatsApp link was sent.
    c) The user confirms they have no further questions.
    Always say a brief, professional closing (e.g., "Take care, goodbye") before the tool executes.

### FNOL STEPS:
1. Confirm Safety. 2. Build Reassurance. 3. Vehicle Reg (MH/KA/DL etc.). 4. Relationship to Policy. 5. Incident Narration (What/Where/When). 6. Fill Gaps. 7. Police/FIR (if injuries). 8. Closing & Reference Number.

Introduce yourself as "Hello, I'm Anika from KIWI Insurance" and ask how you can help.`,
					},
				},
			},
			Tools: []GeminiTool{
				{
					FunctionDeclarations: []GeminiFunctionDeclaration{
						{
							Name:        "call_end",
							Description: "Ends the current phone call immediately. Trigger this when the conversation is finished or the user wants to hang up.",
							Parameters: map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"callId": map[string]interface{}{
										"type":        "string",
										"description": "The unique identifier for the call session.",
									},
								},
								"required": []string{"callId"},
							},
						},
						{
							Name:        "get_customer_info",
							Description: "Retrieves the user's name, age, and address from the database.",
							Parameters: map[string]interface{}{
								"type":       "object",
								"properties": map[string]interface{}{},
							},
						},
					},
				},
			},
		},
	}

	if err := geminiWs.WriteJSON(setupMsg); err != nil {
		log.Println("❌ Error sending setup message:", err)
		return err
	}
	log.Println("📤 Session configuration sent with Gemini 2.0 Flash (Google AI API)")

	// Rest of the implementation is identical to the main file
	// ... (same goroutines for handling Gemini -> Vobiz and Vobiz -> Gemini)

	done := make(chan struct{})
	modelSpeaking := false
	userInputBuffer := ""
	fmt.Println(modelSpeaking)

	// --- Goroutine A: Gemini -> Vobiz (Speaking) ---
	go func() {
		defer close(done)
		for {
			_, rawMsg, err := geminiWs.ReadMessage()
			if err != nil {
				log.Println("❌ Error reading from Gemini:", err)
				return
			}

			// fmt.Printf("Raw Message: %v", string(rawMsg))

			var msg GeminiServerMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				log.Printf("❌ Error parsing Gemini message: %v", err)
				continue
			}

			if msg.SetupComplete != nil {
				log.Printf("✅ Session Configured Successfully with Gemini 2.0 Flash (Session ID: %s)", msg.SetupComplete.SessionID)
				// Trigger the first message from the AI
				initialGreeting := GeminiClientMessage{
					ClientContent: &GeminiClientContent{
						Turns: []GeminiContent{
							{
								Role: "user",
								Parts: []GeminiPart{
									{Text: "Please start the call by introducing yourself as per your instructions."},
								},
							},
						},
						TurnComplete: true,
					},
				}

				if err := geminiWs.WriteJSON(initialGreeting); err != nil {
					log.Printf("❌ Error sending initial greeting trigger: %v", err)
				}

				continue
			}

			if msg.ServerContent != nil {
				if msg.ServerContent.Interrupted {
					log.Println("🎤 User interrupted - Clearing Vobiz buffer")
					err = vobizWs.WriteJSON(VobizOutboundMessage{Event: "clearAudio"})
					if err != nil {
						log.Printf("❌ Error clearing Vobiz buffer: %v", err)
					}
					modelSpeaking = false
				}

				if msg.ServerContent.ModelTurn != nil {
					modelSpeaking = true
					for _, part := range msg.ServerContent.ModelTurn.Parts {
						if part.InlineData != nil && part.InlineData.MimeType == "audio/pcm" {
							payload := VobizOutboundMessage{
								Event: "playAudio",
								Media: &VobizMedia{
									ContentType: "audio/x-mulaw",
									SampleRate:  8000,
									Payload:     part.InlineData.Data,
								},
							}
							if err := vobizWs.WriteJSON(payload); err != nil {
								log.Printf("❌ Error sending audio to Vobiz: %v", err)
							}
						}

						if part.Text != "" {
							log.Printf("🤖 AI says: %s", part.Text)
							if msg.ServerContent.TurnComplete {
								trans := models.TranscriptModel{
									Role:    "AI",
									Content: part.Text,
									CallId:  callId,
								}
								rabbitmq.RabbitMQProducer(trans)
							}
						}
					}
				}

				if msg.ServerContent.TurnComplete {
					log.Println("✅ Model turn complete")
					modelSpeaking = false
				}

				if msg.ServerContent.GenerationComplete {
					log.Println("✅ Generation complete")
				}
			}

			if msg.InputTranscription != nil {
				if msg.InputTranscription.Text != "" {
					userInputBuffer += msg.InputTranscription.Text
					log.Printf("👤 User speaking: %s", msg.InputTranscription.Text)
				}
				if msg.InputTranscription.Finished {
					log.Printf("👤 USER said (complete): %s", userInputBuffer)
					trans := models.TranscriptModel{
						Role:    "User",
						Content: userInputBuffer,
						CallId:  callId,
					}
					rabbitmq.RabbitMQProducer(trans)
					userInputBuffer = ""
				}
			}

			if msg.OutputTranscription != nil && msg.OutputTranscription.Text != "" {
				log.Printf("🤖 AI transcription: %s", msg.OutputTranscription.Text)
			}

			if msg.ToolCall != nil {
				for _, fnCall := range msg.ToolCall.FunctionCalls {
					log.Printf("🛠️ Tool Call: %s (ID: %s) with args: %v", fnCall.Name, fnCall.ID, fnCall.Args)

					var toolOutput map[string]interface{}

					if fnCall.Name == "get_customer_info" {
						toolOutput = getCustomerInfo()
					} else if fnCall.Name == "call_end" {
						targetID, _ := fnCall.Args["callId"].(string)
						if targetID == "" {
							targetID = callId
						}

						err := callEnd(targetID)
						if err != nil {
							toolOutput = map[string]interface{}{"error": err.Error()}
						} else {
							toolOutput = map[string]interface{}{"status": "call_terminated"}
						}
					}

					responseMsg := GeminiClientMessage{
						ToolResponse: &GeminiToolResponse{
							FunctionResponses: []GeminiFunctionResponse{
								{
									Name:     fnCall.Name,
									ID:       fnCall.ID,
									Response: toolOutput,
								},
							},
						},
					}

					if err := geminiWs.WriteJSON(responseMsg); err != nil {
						log.Printf("❌ Error sending tool response: %v", err)
					} else {
						log.Printf("✅ Tool response sent for %s", fnCall.Name)
					}
				}
			}

			if msg.ToolCallCancellation != nil {
				log.Printf("🚫 Tool calls cancelled: %v", msg.ToolCallCancellation.IDs)
			}
		}
	}()

	// --- Goroutine B: Vobiz -> Gemini (Listening) ---
	for {
		var msg VobizInboundMessage
		err = vobizWs.ReadJSON(&msg)
		if err != nil {
			transcripts := redisClient.GetAllTranscript(callId)
			for _, transcript := range transcripts {
				fmt.Printf("Role: %s \t", transcript.Role)
				fmt.Printf("Content: %s \t", transcript.Content)
				fmt.Printf("CallId: %s \n", transcript.CallId)
			}

			log.Println("🛑 Vobiz connection closed:", err)
			break
		}

		switch msg.Event {
		case "start":
			callId = msg.Start.CallId
			log.Printf("Full Message Received: %+v", msg)
			log.Printf("📞 Call Started (SID: %s) (CallID: %s)", msg.Start.StreamId, msg.Start.CallId)
			time.Sleep(500 * time.Millisecond)

		case "media":
			if msg.Media.Payload != "" {
				audioData, err := base64.StdEncoding.DecodeString(msg.Media.Payload)
				if err != nil {
					log.Printf("❌ Error decoding audio: %v", err)
					continue
				}

				// TODO: Convert mu-law to PCM here
				pcmBase64 := base64.StdEncoding.EncodeToString(audioData)

				realtimeMsg := GeminiClientMessage{
					RealtimeInput: &GeminiRealtimeInput{
						MediaChunks: []GeminiBlob{
							{
								MimeType: "audio/pcm",
								Data:     pcmBase64,
							},
						},
					},
				}

				if err := geminiWs.WriteJSON(realtimeMsg); err != nil {
					log.Printf("❌ Error sending audio to Gemini: %v", err)
				}
			}

		case "stop":
			log.Println("🛑 Stream Stopped by Vobiz")
			return nil
		}
	}

	return nil
}
