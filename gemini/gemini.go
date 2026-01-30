package gemini

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

// --- Structs for Vobiz Messages (unchanged) ---

type VobizInboundMessage struct {
	Event    string `json:"event"`
	StreamID string `json:"streamId"`
	Start    struct {
		CallId    string `json:"callId"`
		StreamId  string `json:"streamId"`
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

// --- Gemini Live API Structs ---

// Client -> Gemini Messages
type GeminiClientMessage struct {
	Setup         *GeminiSetup         `json:"setup,omitempty"`
	ClientContent *GeminiClientContent `json:"clientContent,omitempty"`
	RealtimeInput *GeminiRealtimeInput `json:"realtimeInput,omitempty"`
	ToolResponse  *GeminiToolResponse  `json:"toolResponse,omitempty"`
}

type GeminiSetup struct {
	Model             string                  `json:"model"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	Tools             []GeminiTool            `json:"tools,omitempty"`
}

type GeminiGenerationConfig struct {
	ResponseModalities []string            `json:"responseModalities,omitempty"`
	SpeechConfig       *GeminiSpeechConfig `json:"speechConfig,omitempty"`
	Temperature        float64             `json:"temperature,omitempty"`
	TopP               float64             `json:"topP,omitempty"`
	TopK               int                 `json:"topK,omitempty"`
}

type GeminiSpeechConfig struct {
	VoiceConfig *GeminiVoiceConfig `json:"voiceConfig,omitempty"`
}

type GeminiVoiceConfig struct {
	PrebuiltVoiceConfig *GeminiPrebuiltVoiceConfig `json:"prebuiltVoiceConfig,omitempty"`
}

type GeminiPrebuiltVoiceConfig struct {
	VoiceName string `json:"voiceName"`
}

type GeminiClientContent struct {
	Turns        []GeminiContent `json:"turns,omitempty"`
	TurnComplete bool            `json:"turnComplete,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type GeminiRealtimeInput struct {
	MediaChunks []GeminiBlob `json:"mediaChunks,omitempty"`
}

type GeminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64 encoded
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type GeminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type GeminiToolResponse struct {
	FunctionResponses []GeminiFunctionResponse `json:"functionResponses"`
}

type GeminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
	ID       string                 `json:"id,omitempty"`
}

// Gemini -> Client Messages
type GeminiServerMessage struct {
	SetupComplete        *GeminiSetupComplete        `json:"setupComplete,omitempty"`
	ServerContent        *GeminiServerContent        `json:"serverContent,omitempty"`
	ToolCall             *GeminiToolCall             `json:"toolCall,omitempty"`
	ToolCallCancellation *GeminiToolCallCancellation `json:"toolCallCancellation,omitempty"`
	InputTranscription   *GeminiTranscription        `json:"inputTranscription,omitempty"`
	OutputTranscription  *GeminiTranscription        `json:"outputTranscription,omitempty"`
}

type GeminiSetupComplete struct {
	SessionID string `json:"sessionId"`
}

type GeminiServerContent struct {
	ModelTurn          *GeminiContent `json:"modelTurn,omitempty"`
	TurnComplete       bool           `json:"turnComplete,omitempty"`
	Interrupted        bool           `json:"interrupted,omitempty"`
	GenerationComplete bool           `json:"generationComplete,omitempty"`
}

type GeminiToolCall struct {
	FunctionCalls []GeminiFunctionCall `json:"functionCalls"`
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
	ID   string                 `json:"id"`
}

type GeminiToolCallCancellation struct {
	IDs []string `json:"ids"`
}

type GeminiTranscription struct {
	Text     string `json:"text,omitempty"`
	Finished bool   `json:"finished,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// --- Updated WebSocket Handler for Gemini ---

func HandleWebSocketStream(c echo.Context) error {
	// var GeminiProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	// var GeminiLocation = os.Getenv("GOOGLE_CLOUD_LOCATION")
	var GeminiAccessToken = os.Getenv("GOOGLE_ACCESS_TOKEN")
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

	// 2. Connect to Gemini Live API
	// Construct Gemini WebSocket URL
	geminiURL := fmt.Sprintf(
		"wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent?key=%s",
		GeminiAccessToken,
	)

	// For Vertex AI, use this format instead:
	// geminiURL := fmt.Sprintf(
	// 	"wss://%s-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent",
	// 	GeminiLocation,
	// )

	header := http.Header{}
	// For Vertex AI, add authorization header:
	// header.Add("Authorization", "Bearer "+GeminiAccessToken)

	geminiWs, _, err := websocket.DefaultDialer.Dial(geminiURL, header)
	if err != nil {
		log.Printf("❌ Failed to connect to Gemini: %v", err)
		return err
	}
	defer geminiWs.Close()
	log.Println("✅ Gemini Connected")

	// 3. Configure Session
	setupMsg := GeminiClientMessage{
		Setup: &GeminiSetup{
			Model: "gemini-2.0-flash-live-preview-04-22", // or gemini-2.0-flash-live-preview-04-09
			GenerationConfig: &GeminiGenerationConfig{
				ResponseModalities: []string{"AUDIO"},
				SpeechConfig: &GeminiSpeechConfig{
					VoiceConfig: &GeminiVoiceConfig{
						PrebuiltVoiceConfig: &GeminiPrebuiltVoiceConfig{
							VoiceName: "Puck", // Available: Puck, Charon, Kore, Fenrir, Aoede
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
	log.Println("📤 Session configuration sent")

	// Channels to handle graceful shutdown
	done := make(chan struct{})

	// Track if we're currently receiving audio to avoid interrupting ourselves
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

			fmt.Printf("Raw Message: %v", rawMsg)

			var msg GeminiServerMessage
			if err := json.Unmarshal(rawMsg, &msg); err != nil {
				log.Printf("❌ Error parsing Gemini message: %v", err)
				continue
			}

			// Handle SetupComplete
			if msg.SetupComplete != nil {
				log.Printf("✅ Session Configured Successfully (Session ID: %s)", msg.SetupComplete.SessionID)
				continue
			}

			// Handle ServerContent (model's response)
			if msg.ServerContent != nil {
				if msg.ServerContent.Interrupted {
					log.Println("🎤 User interrupted - Clearing Vobiz buffer")
					vobizWs.WriteJSON(VobizOutboundMessage{Event: "clearAudio"})
					modelSpeaking = false
				}

				if msg.ServerContent.ModelTurn != nil {
					modelSpeaking = true
					for _, part := range msg.ServerContent.ModelTurn.Parts {
						// Handle audio output
						if part.InlineData != nil && part.InlineData.MimeType == "audio/pcm" {
							// Gemini sends PCM, need to convert to mu-law for Vobiz
							// For now, we'll send as-is and handle conversion separately
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

						// Handle text output (for logging/transcription)
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

			// Handle Input Transcription (user's speech)
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

			// Handle Output Transcription (AI's speech)
			if msg.OutputTranscription != nil && msg.OutputTranscription.Text != "" {
				log.Printf("🤖 AI transcription: %s", msg.OutputTranscription.Text)
			}

			// Handle Tool Calls
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

					// Send function response back to Gemini
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

			// Handle Tool Call Cancellation
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

			// Wait for setup to complete
			time.Sleep(500 * time.Millisecond)

			// No need to send initial greeting - Gemini will start based on system instruction

		case "media":
			if msg.Media.Payload != "" {
				// Vobiz sends mu-law audio, Gemini expects PCM
				// You'll need to convert mu-law to PCM here
				// For now, sending as-is (you'll need to add conversion)

				// Decode base64 mu-law
				audioData, err := base64.StdEncoding.DecodeString(msg.Media.Payload)
				if err != nil {
					log.Printf("❌ Error decoding audio: %v", err)
					continue
				}

				// TODO: Convert mu-law to PCM here
				// pcmData := convertMuLawToPCM(audioData)

				// For now, re-encode as base64 (replace with converted PCM)
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

func callEnd(callId string) error {
	var VobizAuthID = os.Getenv("VOBIZ_AUTH_ID")
	var VobizAuthToken = os.Getenv("VOBIZ_AUTH_TOKEN")

	url := fmt.Sprintf("https://api.vobiz.ai/api/v1/Account/%s/Call/%s/", VobizAuthID, callId)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Auth-ID", VobizAuthID)
	req.Header.Set("X-Auth-Token", VobizAuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

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
		"address": "Village Bhoya, Post Harsh, Sikar, Rajasthan, India, 332021",
	}
}
