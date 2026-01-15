package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/labstack/echo/v4"
)

// --- Structs for Request/Response ---

// OutboundCallRequest represents the incoming JSON body from your client
type OutboundCallRequest struct {
	FromNumber string                 `json:"from_number"`
	ToNumber   string                 `json:"to_number"`
	Body       map[string]interface{} `json:"body"` // Optional extra data
}

// VobizCallPayload represents the payload sent to Vobiz API
type VobizCallPayload struct {
	From         string `json:"from"`
	To           string `json:"to"`
	AnswerURL    string `json:"answer_url"`
	AnswerMethod string `json:"answer_method"`
}

// --- Configuration ---

var (
	VobizBaseURL = "https://api.vobiz.ai/api/v1/Account"
)

// --- Handler ---

// HandleOutboundCall initiates a call via Vobiz
func HandleOutboundCall(c echo.Context) error {
	var VobizAuthID = os.Getenv("VOBIZ_AUTH_ID")
	var VobizAuthToken = os.Getenv("VOBIZ_AUTH_TOKEN")
	log.Println("[Vobiz] outbound call api is triggered")

	// 1. Parse Incoming Request
	req := new(OutboundCallRequest)
	if err := c.Bind(req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid JSON body")
	}

	// 2. Validate Inputs
	if req.FromNumber == "" || req.ToNumber == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing 'from_number' or 'to_number' in request body")
	}

	log.Printf("[DEBUG] Body data: %v\n", req.Body)

	// 3. Construct Answer URL
	// We need to determine the protocol (http/https) and host dynamically
	scheme := c.Scheme()
	if scheme == "" {
		scheme = "http" // Default fallback
		if c.Request().TLS != nil {
			scheme = "https"
		}
	}
	host := c.Request().Host // e.g., "example.com" or "localhost:8080"

	// Base Answer URL
	answerURL := fmt.Sprintf("%s://%s/incoming-call", scheme, host)

	// Append encoded body data if present
	if len(req.Body) > 0 {
		jsonData, err := json.Marshal(req.Body)
		if err == nil {
			// URL Encode the JSON string
			encodedData := url.QueryEscape(string(jsonData))
			answerURL = fmt.Sprintf("%s?body_data=%s", answerURL, encodedData)
		} else {
			log.Printf("[WARN] Failed to marshal body data: %v", err)
		}
	}

	log.Printf("[INFO] Answer URL that will be sent to Vobiz: %s", answerURL)

	// 4. Prepare Request to Vobiz
	vobizURL := fmt.Sprintf("%s/%s/Call/", VobizBaseURL, VobizAuthID)

	payload := VobizCallPayload{
		From:         req.FromNumber,
		To:           req.ToNumber,
		AnswerURL:    answerURL,
		AnswerMethod: "POST",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to marshal payload")
	}

	// 5. Send HTTP POST to Vobiz
	client := &http.Client{Timeout: 10 * time.Second}
	vobizReq, err := http.NewRequest("POST", vobizURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create request")
	}

	// Add Headers
	vobizReq.Header.Set("X-Auth-ID", VobizAuthID)
	vobizReq.Header.Set("X-Auth-Token", VobizAuthToken)
	vobizReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(vobizReq)
	if err != nil {
		log.Printf("[ERROR] HTTP request error: %v", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("Vobiz API request failed: %v", err))
	}
	defer resp.Body.Close()

	// 6. Handle Response
	var vobizResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&vobizResp); err != nil {
		log.Printf("[ERROR] Failed to decode Vobiz response")
	}

	if resp.StatusCode != 201 {
		log.Printf("[ERROR] Vobiz API call failed with status %d: %v", resp.StatusCode, vobizResp)
		return echo.NewHTTPError(resp.StatusCode, fmt.Sprintf("Vobiz API error: %v", vobizResp))
	}

	log.Println("[SUCCESS] Vobiz API call successful!", vobizResp)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    vobizResp,
	})
}
