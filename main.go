package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Configuration
const (
	OpenAIRealtimeURL = "wss://api.openai.com/v1/realtime?model=gpt-realtime-mini"
	ServerPort        = ":8080"
)

var (
	// Standard Gorilla WebSocket upgrader
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 1. Validates Vobiz is connecting and returns XML
	e.POST("/incoming-call", HandleIncomingCall)

	// 2. The WebSocket Bridge
	e.GET("/stream", HandleWebSocketStream)
	e.POST("/hangup", handleHangup)
	e.POST("/outbound-call", HandleOutboundCall)

	e.Logger.Fatal(e.Start(ServerPort))
}

// handleIncomingCall returns the XML telling Vobiz to connect to our WebSocket
func handleHangup(c echo.Context) error {
	fmt.Println("Hangup called")
	var body map[string]interface{}

	if err := c.Bind(&body); err != nil {
		return err
	}

	fmt.Println(body)

	return nil

}
