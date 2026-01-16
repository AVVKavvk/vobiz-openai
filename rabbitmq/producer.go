package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/AVVKavvk/openai-vobiz/models"
	"github.com/rabbitmq/amqp091-go"
)

func RabbitMQProducer(body models.TranscriptModel) {
	ch, _ := RabbitMQConn.Channel()
	defer ch.Close()

	err := ch.ExchangeDeclare(Transcript, "direct", true, false, false, false, nil)

	if err != nil {
		panic(err)
	}

	bodystr, err := json.Marshal(body)
	if err != nil {
		// Handle the error (e.g., return it or log it)
		fmt.Println("Error encoding JSON:", err)
		return
	}
	err = ch.PublishWithContext(context.Background(), Transcript, "", false, false, amqp091.Publishing{ContentType: "text/plan", Body: bodystr})
	if err != nil {
		panic(err)
	}
	log.Printf(" [x] Sent %s", body)
}
