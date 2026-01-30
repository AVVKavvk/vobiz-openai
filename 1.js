const WebSocket = require("ws");

const ws = new WebSocket(
  "ws://localhost:8080/stream?calluuid=test-123&from=918504074217&to=08071387304",
);

ws.on("open", function open() {
  console.log("✅ Connected to WebSocket");

  // 1. Send START event
  const startEvent = {
    event: "start",
    start: {
      callId: "test-call-123",
      streamId: "test-stream-456",
      accountId: "test-account-789",
    },
  };

  console.log("📤 Sending START event");
  ws.send(JSON.stringify(startEvent));

  // 2. Wait a bit, then send media events
  setTimeout(() => {
    console.log("📤 Sending MEDIA events");

    // Send 20 media packets (simulating ~400ms of audio at 20ms intervals)
    let count = 0;
    const interval = setInterval(() => {
      const mediaEvent = {
        event: "media",
        media: {
          payload:
            "//79/f3+/v8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A/wD/AP8A",
        },
      };

      ws.send(JSON.stringify(mediaEvent));
      count++;

      if (count >= 100) {
        clearInterval(interval);

        // 3. Send STOP event after all media
        setTimeout(() => {
          console.log("📤 Sending STOP event");
          ws.send(JSON.stringify({ event: "stop" }));

          setTimeout(() => {
            ws.close();
          }, 1000);
        }, 2000);
      }
    }, 20); // 20ms intervals
  }, 1000); // Wait 1 second after START
});

ws.on("message", function message(data) {
  console.log("📥 Received:", data.toString());
});

ws.on("error", function error(err) {
  console.error("❌ Error:", err);
});

ws.on("close", function close(code, reason) {
  console.log(`🛑 Disconnected (code: ${code}, reason: ${reason})`);
});
