package gemini20

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/AVVKavvk/openai-vobiz/models"
	"github.com/AVVKavvk/openai-vobiz/rabbitmq"
	"github.com/AVVKavvk/openai-vobiz/redisClient"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	modelSpeaking   = false
	modelSpeakingMu sync.Mutex
)

func HandleWebSocketStreamGoogleAI(c echo.Context) error {
	var GeminiAPIKey = os.Getenv("GEMINI_API_KEY")
	var callId string

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

	// 2. Connect to Gemini Live API
	geminiURL := fmt.Sprintf(
		"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s",
		GeminiAPIKey,
	)

	header := http.Header{}
	geminiWs, _, err := websocket.DefaultDialer.Dial(geminiURL, header)
	if err != nil {
		log.Printf("❌ Failed to connect to Gemini: %v", err)
		return err
	}
	defer geminiWs.Close()
	log.Println("✅ Gemini Connected via Google AI API")

	// 3. Configure Session
	setupMsg := GeminiClientMessage{
		Setup: &GeminiSetup{
			Model: "models/gemini-2.5-flash-native-audio-preview-12-2025",
			GenerationConfig: &GeminiGenerationConfig{
				ResponseModalities: []string{"AUDIO"},
				SpeechConfig: &GeminiSpeechConfig{
					VoiceConfig: &GeminiVoiceConfig{
						PrebuiltVoiceConfig: &GeminiPrebuiltVoiceConfig{
							VoiceName: "Puck",
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

IMMEDIATELY greet the caller when the call starts by saying: "Hello, I'm Anika from KIWI Insurance. How can I help you today?"

### CORE POLICIES:
1. ZERO-REPETITION: Never repeat customer details. Use "Recorded" or "I have that noted" and move on.
2. ONE QUESTION AT A TIME: Keep responses short and focused.
3. SAFETY FIRST: Always confirm safety before data collection.

### FUNCTION CALLING PROTOCOLS:
- **get_customer_info**: Call this immediately if the user asks "What information do you have on me?" or if you need to verify their identity/address to proceed with the claim.
- **call_end**: Trigger this tool ONLY when:
    a) The customer says goodbye or indicates they want to hang up.
    b) You have provided the Claim Reference Number (#123098) and confirmed the WhatsApp link was sent.
    c) The user confirms they have no further questions.
    Always say a brief, professional closing (e.g., "Take care, goodbye") before the tool executes.

### FNOL STEPS:
1. Confirm Safety. 2. Build Reassurance. 3. Vehicle Reg (MH/KA/DL etc.). 4. Relationship to Policy. 5. Incident Narration (What/Where/When). 6. Fill Gaps. 7. Police/FIR (if injuries). 8. Closing & Reference Number.`,
					},
				},
			},
			// ✅ ADD THIS - Enables input audio transcription
			InputAudioTranscription: &AudioTranscriptionConfig{},

			// ✅ ADD THIS - Enables output audio transcription (optional but helpful for debugging)
			OutputAudioTranscription: &AudioTranscriptionConfig{},

			// ✅ ADD THIS - Configure voice activity detection
			RealtimeInputConfig: &RealtimeInputConfig{
				AutomaticActivityDetection: &AutomaticActivityDetection{
					Disabled:                 false,
					StartOfSpeechSensitivity: "START_SENSITIVITY_HIGH", // Keep this
					PrefixPaddingMs:          300,                      // Increase from 200
					EndOfSpeechSensitivity:   "END_SENSITIVITY_HIGH",   // Change from LOW to HIGH
					SilenceDurationMs:        500,                      // Increase from
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
	// ADD THIS DEBUG LOG
	setupJSON, _ := json.MarshalIndent(setupMsg, "", "  ")
	log.Printf("📋 Setup message sent:\n%s", string(setupJSON))
	log.Println("📤 Session configuration sent")
	log.Println("📤 Session configuration sent")

	done := make(chan struct{})
	setupComplete := make(chan bool, 1)

	userInputBuffer := ""

	// --- Goroutine A: Gemini -> Vobiz (Speaking) ---
	go func() {
		defer close(done)
		for {
			_, rawMsg, err := geminiWs.ReadMessage()
			if err != nil {
				log.Println("❌ Error reading from Gemini:", err)
				return
			}
			// ✅ ADD THIS - Log EVERY message from Gemini
			log.Printf("📥 Raw Gemini message: %s", string(rawMsg))

			var msg GeminiServerMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				log.Printf("❌ Error parsing Gemini message: %v", err)
				continue
			}

			if msg.SetupComplete != nil {
				log.Println("✅ Setup complete, sending greeting trigger")
				setupComplete <- true
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
					modelSpeakingMu.Lock()
					if !modelSpeaking {
						modelSpeaking = true
						log.Println("🗣️ Model started speaking")
					}
					modelSpeakingMu.Unlock()
					for _, part := range msg.ServerContent.ModelTurn.Parts {
						// Skip thought parts
						if part.Thought {
							log.Printf("💭 AI thinking: %s", part.Text)
							continue
						}

						if part.InlineData != nil {
							// Decode base64 audio
							// Decode base64
							pcm24k, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
							if err != nil {
								continue
							}

							// Convert bytes to int16
							pcmSamples24k := bytesToInt16(pcm24k)

							// Downsample 24kHz → 8kHz
							pcmSamples8k := resample24to8(pcmSamples24k)

							// Convert int16 to bytes for pcmToMuLaw
							pcmBytes8k := int16ToBytes(pcmSamples8k)

							// Convert to μ-law
							mulaw := pcmToMuLaw(pcmBytes8k)

							// Encode to base64
							mulawBase64 := base64.StdEncoding.EncodeToString(mulaw)

							payload := VobizOutboundMessage{
								Event: "playAudio",
								Media: &VobizMedia{
									ContentType: "audio/x-mulaw",
									SampleRate:  8000,
									Payload:     mulawBase64,
								},
							}
							if err := vobizWs.WriteJSON(payload); err != nil {
								log.Printf("❌ Error sending audio to Vobiz: %v", err)
							} else {
								log.Printf("🔊 Sent %d bytes of audio to Vobiz", len(mulaw))
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
					modelSpeakingMu.Lock()
					modelSpeaking = false
					modelSpeakingMu.Unlock()

				}
				if msg.ServerContent.OutputTranscription != nil {
					log.Printf("🤖 AI transcription: %s", msg.ServerContent.OutputTranscription.Text)
				}
				if msg.ServerContent.OutputTranscription != nil {
					log.Printf("👤 User speaking: %s", msg.ServerContent.InputTranscription.Text)
				}
				if msg.ServerContent.Interrupted {
					log.Println("🎤 User interrupted - Clearing Vobiz buffer")
					err = vobizWs.WriteJSON(VobizOutboundMessage{Event: "clearAudio"})
					if err != nil {
						log.Printf("❌ Error clearing Vobiz buffer: %v", err)
					}
					modelSpeakingMu.Lock()
					modelSpeaking = false
					modelSpeakingMu.Unlock()
				}

				if msg.ServerContent.GenerationComplete {
					log.Println("✅ Generation complete")
					modelSpeakingMu.Lock()
					modelSpeaking = false
					modelSpeakingMu.Unlock()
					log.Println("🎧 Model finished generating - listening mode")
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
				log.Printf("🤖 AI Old transcription: %s", msg.OutputTranscription.Text)
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
			if msg.UsageMetadata != nil {
				log.Printf("User Metadata: %v", msg.UsageMetadata)
			}
			if msg.GoAway != nil {
				log.Printf("👋 User disconnected: %v", msg.GoAway)
			}
			if msg.SessionResumptionUpdate != nil {
				log.Printf("Session Resumption Update: %v", msg.SessionResumptionUpdate)
			}
		}
	}()

	// Wait for setup to complete before processing audio
	select {
	case <-setupComplete:
		// Send greeting trigger
		greeting := GeminiClientMessage{
			ClientContent: &GeminiClientContent{
				Turns: []GeminiContent{
					{
						Role: "user",
						Parts: []GeminiPart{
							{Text: "[Call connected. Please greet the caller.]"},
						},
					},
				},
				TurnComplete: true,
			},
		}

		if err := geminiWs.WriteJSON(greeting); err != nil {
			log.Printf("❌ Error sending greeting trigger: %v", err)
		} else {
			log.Println("📤 Greeting trigger sent successfully")
		}
	case <-time.After(5 * time.Second):
		log.Println("⚠️ Timeout waiting for setup complete")
		return fmt.Errorf("setup timeout")
	}

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
			log.Printf("📞 Call Started (SID: %s) (CallID: %s)", msg.Start.StreamId, msg.Start.CallId)

		case "media":
			if msg.Media.Payload != "" {
				modelSpeakingMu.Lock()
				isSpeaking := modelSpeaking
				modelSpeakingMu.Unlock()

				if isSpeaking {
					// Skip sending audio while AI is speaking
					continue
				}

				mulawData, _ := base64.StdEncoding.DecodeString(msg.Media.Payload)
				log.Printf("📥 Received %d bytes of μ-law from Vobiz", len(mulawData))

				pcm8k := muLawToPCM(mulawData)
				log.Printf("🔄 Converted to %d bytes of PCM 8kHz", len(pcm8k))

				// Verify PCM has actual audio data (not silence)
				hasAudio := false
				for i := 0; i < len(pcm8k) && i < 100; i += 2 {
					sample := int16(pcm8k[i]) | int16(pcm8k[i+1])<<8
					if sample > 100 || sample < -100 {
						hasAudio = true
						break
					}
				}
				log.Printf("🎵 Audio content detected: %v", hasAudio)

				pcm24k := upsample8to24(pcm8k)
				log.Printf("⬆️ Upsampled to %d bytes of PCM 24kHz", len(pcm24k))

				pcmBase64 := base64.StdEncoding.EncodeToString(pcm24k)

				realtimeMsg := GeminiClientMessage{
					RealtimeInput: &GeminiRealtimeInput{
						Audio: &GeminiBlob{
							MimeType: "audio/pcm;rate=24000",
							Data:     pcmBase64,
						},
					},
				}

				if err := geminiWs.WriteJSON(realtimeMsg); err != nil {
					log.Printf("❌ Error sending audio to Gemini: %v", err)
				} else {
					log.Printf("🎤 Sent %d bytes of PCM audio (24kHz) to Gemini", len(pcm24k))
				}
			}

		case "stop":
			log.Println("🛑 Stream Stopped by Vobiz")
			return nil
		}
	}

	return nil
}
