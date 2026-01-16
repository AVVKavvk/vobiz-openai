package rabbitmq

import (
	"encoding/json"
	"log"

	"github.com/AVVKavvk/openai-vobiz/models"
	"github.com/AVVKavvk/openai-vobiz/redisClient"
)

func RabbitMQConsumer() {

	ch, _ := RabbitMQConn.Channel()

	err := ch.ExchangeDeclare(Transcript, "direct", true, false, false, false, nil)

	if err != nil {
		panic(err)
	}
	q, err := ch.QueueDeclare("", false, false, true, false, nil)

	if err != nil {
		panic(err)
	}

	err = ch.QueueBind(q.Name, "", Transcript, false, nil)

	if err != nil {
		panic(err)
	}

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)

	if err != nil {
		panic(err)
	}
	log.Printf(" [*] Waiting for logs. To exit press CTRL+C")
	for d := range msgs {
		var transcript models.TranscriptModel
		err := json.Unmarshal(d.Body, &transcript)
		if err != nil {
			panic(err)
		}
		log.Printf(" [x] %s", d.Body)

		err = redisClient.
			AppendTranscript(transcript, transcript.CallId)

		if err != nil {
			panic(err)
		}
	}

}
