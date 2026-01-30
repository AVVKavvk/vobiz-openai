package gemini20

import "log"

// Gemini Message Structs
type GeminiSetup struct {
	Model                    string                    `json:"model"`
	GenerationConfig         *GeminiGenerationConfig   `json:"generationConfig,omitempty"`
	SystemInstruction        *GeminiContent            `json:"systemInstruction,omitempty"`
	Tools                    []GeminiTool              `json:"tools,omitempty"`
	RealtimeInputConfig      *RealtimeInputConfig      `json:"realtimeInputConfig,omitempty"`
	InputAudioTranscription  *AudioTranscriptionConfig `json:"inputAudioTranscription,omitempty"`
	OutputAudioTranscription *AudioTranscriptionConfig `json:"outputAudioTranscription,omitempty"`
}
type GeminiGenerationConfig struct {
	ResponseModalities []string            `json:"responseModalities,omitempty"`
	SpeechConfig       *GeminiSpeechConfig `json:"speechConfig,omitempty"`
	Temperature        float64             `json:"temperature,omitempty"`
	TopP               float64             `json:"topP,omitempty"`
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

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *GeminiBlob `json:"inlineData,omitempty"`
	Thought    bool        `json:"thought,omitempty"`
}

type GeminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations"`
}

type GeminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type GeminiClientMessage struct {
	Setup         *GeminiSetup         `json:"setup,omitempty"`
	ClientContent *GeminiClientContent `json:"clientContent,omitempty"`
	RealtimeInput *GeminiRealtimeInput `json:"realtimeInput,omitempty"`
	ToolResponse  *GeminiToolResponse  `json:"toolResponse,omitempty"`
}

type GeminiClientContent struct {
	Turns        []GeminiContent `json:"turns,omitempty"`
	TurnComplete bool            `json:"turnComplete,omitempty"`
}

type GeminiRealtimeInput struct {
	MediaChunks    []GeminiBlob `json:"mediaChunks,omitempty"`    // Deprecated but keep for compatibility
	Audio          *GeminiBlob  `json:"audio,omitempty"`          // Use this for audio
	Video          *GeminiBlob  `json:"video,omitempty"`          // For video input
	Text           string       `json:"text,omitempty"`           // For text input
	ActivityStart  interface{}  `json:"activityStart,omitempty"`  // Marks start of user activity
	ActivityEnd    interface{}  `json:"activityEnd,omitempty"`    // Marks end of user activity
	AudioStreamEnd bool         `json:"audioStreamEnd,omitempty"` // Indicates audio stream ended
}
type GeminiToolResponse struct {
	FunctionResponses []GeminiFunctionResponse `json:"functionResponses"`
}

type GeminiFunctionResponse struct {
	Name     string                 `json:"name"`
	ID       string                 `json:"id"`
	Response map[string]interface{} `json:"response"`
}

type GeminiServerMessage struct {
	SetupComplete           *GeminiSetupComplete        `json:"setupComplete,omitempty"`
	ServerContent           *GeminiServerContent        `json:"serverContent,omitempty"`
	ToolCall                *GeminiToolCall             `json:"toolCall,omitempty"`
	ToolCallCancellation    *GeminiToolCallCancellation `json:"toolCallCancellation,omitempty"`
	InputTranscription      *GeminiTranscription        `json:"inputTranscription,omitempty"`
	OutputTranscription     *GeminiTranscription        `json:"outputTranscription,omitempty"`
	UsageMetadata           *GeminiUsageMetadata        `json:"usageMetadata,omitempty"`
	GoAway                  *GeminiGoAway               `json:"goAway,omitempty"`
	SessionResumptionUpdate *GeminiResumption           `json:"sessionResumptionUpdate,omitempty"`
}

type GeminiUsageMetadata struct {
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
	TextCount            int `json:"textCount,omitempty"`
	ImageCount           int `json:"imageCount,omitempty"`
	VideoDurationSeconds int `json:"videoDurationSeconds,omitempty"`
	AudioDurationSeconds int `json:"audioDurationSeconds,omitempty"`
}

type GeminiGoAway struct {
	TimeLeft interface{} `json:"timeLeft"`
}

type GeminiResumption struct {
	NewHandle                      string `json:"newHandle"`
	Resumable                      bool   `json:"resumable"`
	LastConsumedClientMessageIndex int    `json:"lastConsumedClientMessageIndex"`
}
type GeminiSetupComplete struct {
	SessionID string `json:"sessionId,omitempty"`
}

type GeminiServerContent struct {
	ModelTurn           *GeminiContent       `json:"modelTurn,omitempty"`
	TurnComplete        bool                 `json:"turnComplete,omitempty"`
	Interrupted         bool                 `json:"interrupted,omitempty"`
	GenerationComplete  bool                 `json:"generationComplete,omitempty"`
	InputTranscription  *GeminiTranscription `json:"inputTranscription,omitempty"`
	OutputTranscription *GeminiTranscription `json:"outputTranscription,omitempty"`
}

type GeminiToolCall struct {
	FunctionCalls []GeminiFunctionCall `json:"functionCalls"`
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	ID   string                 `json:"id"`
	Args map[string]interface{} `json:"args"`
}

type GeminiToolCallCancellation struct {
	IDs []string `json:"ids"`
}

type GeminiTranscription struct {
	Text     string `json:"text,omitempty"`
	Finished bool   `json:"finished,omitempty"`
}

// Vobiz Message Structs
type VobizInboundMessage struct {
	Event    string      `json:"event"`
	StreamID string      `json:"streamSid,omitempty"`
	Start    *VobizStart `json:"start,omitempty"`
	Media    *VobizMedia `json:"media,omitempty"`
}

type VobizStart struct {
	CallId    string `json:"callId"`
	StreamId  string `json:"streamId"`
	AccountId string `json:"accountId"`
}

type VobizMedia struct {
	Payload     string `json:"payload"`
	ContentType string `json:"contentType,omitempty"`
	SampleRate  int    `json:"sampleRate,omitempty"`
}

type VobizOutboundMessage struct {
	Event string      `json:"event"`
	Media *VobizMedia `json:"media,omitempty"`
}

type RealtimeInputConfig struct {
	AutomaticActivityDetection *AutomaticActivityDetection `json:"automaticActivityDetection,omitempty"`
}

type AutomaticActivityDetection struct {
	Disabled                 bool   `json:"disabled,omitempty"`
	StartOfSpeechSensitivity string `json:"startOfSpeechSensitivity,omitempty"`
	PrefixPaddingMs          int32  `json:"prefixPaddingMs,omitempty"`
	EndOfSpeechSensitivity   string `json:"endOfSpeechSensitivity,omitempty"`
	SilenceDurationMs        int32  `json:"silenceDurationMs,omitempty"`
}

type AudioTranscriptionConfig struct {
	// Empty struct - no fields needed according to API docs
}

// μ-law to PCM conversion
var muLawTable = buildMuLawTable()

func buildMuLawTable() [256]int16 {
	var table [256]int16
	for i := 0; i < 256; i++ {
		mulaw := byte(i)
		sign := (mulaw & 0x80)
		exponent := (mulaw >> 4) & 0x07
		mantissa := mulaw & 0x0F

		sample := int16(mantissa)<<3 + 0x84
		sample <<= exponent
		sample -= 0x84

		if sign != 0 {
			sample = -sample
		}
		table[i] = sample
	}
	return table
}

func muLawToPCM(mulaw []byte) []byte {
	pcm := make([]byte, len(mulaw)*2)
	for i, mu := range mulaw {
		sample := muLawTable[mu]
		pcm[i*2] = byte(sample & 0xFF)
		pcm[i*2+1] = byte((sample >> 8) & 0xFF)
	}
	return pcm
}

// Convert byte slice to int16 slice
func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}
func upsample8to24(pcm8k []byte) []byte {
	samples8k := len(pcm8k) / 2
	pcm24k := make([]byte, samples8k*3*2) // Triple the samples (8kHz -> 24kHz)

	for i := 0; i < samples8k; i++ {
		// Get original sample (16-bit little-endian)
		sample := int16(pcm8k[i*2]) | int16(pcm8k[i*2+1])<<8

		// Write sample three times (simple repeat, not interpolation)
		for j := 0; j < 3; j++ {
			offset := (i*3 + j) * 2
			pcm24k[offset] = byte(sample & 0xFF)
			pcm24k[offset+1] = byte((sample >> 8) & 0xFF)
		}
	}

	return pcm24k
}

// Convert int16 slice to byte slice
func int16ToBytes(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		data[i*2] = byte(sample)
		data[i*2+1] = byte(sample >> 8)
	}
	return data
}

func resample24to8(input []int16) []int16 {
	output := make([]int16, len(input)/3) // Downsample by 3x
	for i := 0; i < len(output); i++ {
		output[i] = input[i*3]
	}
	return output
}

// PCM to μ-law conversion (for Gemini -> Vobiz)
func pcmToMuLaw(pcm []byte) []byte {
	mulaw := make([]byte, len(pcm)/2)
	for i := 0; i < len(pcm); i += 2 {
		sample := int16(pcm[i]) | int16(pcm[i+1])<<8
		mulaw[i/2] = linearToMuLaw(sample)
	}
	return mulaw
}

func linearToMuLaw(sample int16) byte {
	const BIAS = 0x84
	const CLIP = 32635

	sign := byte(0)
	if sample < 0 {
		sample = -sample
		sign = 0x80
	}

	if sample > CLIP {
		sample = CLIP
	}
	sample += BIAS

	exponent := byte(7)
	for mask := int16(0x4000); (sample&mask) == 0 && exponent > 0; exponent-- {
		mask >>= 1
	}

	mantissa := byte((sample >> (exponent + 3)) & 0x0F)
	mulaw := ^(sign | (exponent << 4) | mantissa)

	return mulaw
}

// Tool functions
func getCustomerInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":    "John Doe",
		"age":     35,
		"address": "123 Main St, Mumbai, India",
	}
}

func callEnd(callId string) error {
	log.Printf("🔴 Ending call: %s", callId)
	// Implement actual call termination logic here
	return nil
}
