package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func HandleIncomingCall(c echo.Context) error {
	// 1. Extract the specific parameters from the POST body
	callUUID := c.FormValue("CallUUID")
	from := c.FormValue("From")
	to := c.FormValue("To")

	// Log them for debugging
	log.Printf("Call Received - UUID: %s, From: %s, To: %s", callUUID, from, to)

	// 2. Construct the WebSocket URL
	host := c.Request().Host
	baseURL := fmt.Sprintf("wss://%s/stream", host)

	// You can manually append the specific params to the WebSocket URL
	fullURL := fmt.Sprintf("%s?calluuid=%s&from=%s&to=%s", baseURL, callUUID, from, to)

	// 3. Escape & for XML
	finalURLForXML := strings.ReplaceAll(fullURL, "&", "&amp;")

	// 4. Generate XML Response
	xmlResponse := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<Stream bidirectional="true" keepCallAlive="true" contentType="audio/x-mulaw;rate=8000">%s</Stream>
</Response>`, finalURLForXML)

	return c.Blob(http.StatusOK, "application/xml", []byte(xmlResponse))
}
