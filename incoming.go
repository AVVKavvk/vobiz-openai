package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func HandleIncomingCall(c echo.Context) error {
	// ... (parsing logic remains the same) ...

	// 1. Construct the WebSocket URL
	host := c.Request().Host
	baseURL := fmt.Sprintf("wss://%s/stream", host)
	queryString := c.Request().URL.RawQuery // This will include the 'from_number' and 'to_number' params.Encode()

	var fullURL string
	if queryString != "" {
		fullURL = fmt.Sprintf("%s?%s", baseURL, queryString)
	} else {
		fullURL = baseURL
	}

	// 2. Escape & for XML
	finalURLForXML := strings.ReplaceAll(fullURL, "&", "&amp;")

	log.Printf("WSS URL %s", fullURL)

	// 3. STRICT XML FORMATTING (No spaces inside Stream!)
	xmlResponse := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
<Stream bidirectional="true" keepCallAlive="true" contentType="audio/x-mulaw;rate=8000">%s</Stream>
</Response>`, finalURLForXML)

	// Set the Content-Type explicitly to application/xml
	return c.Blob(http.StatusOK, "application/xml", []byte(xmlResponse))
}
