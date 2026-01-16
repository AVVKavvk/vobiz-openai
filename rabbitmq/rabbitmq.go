package rabbitmq

import (
	"log"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	RabbitMQConn *amqp.Connection
	once         sync.Once
	Transcript   string
)

func init() {
	once.Do(func() {
		var err error
		conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
		if err != nil {
			panic(err)
		}
		log.Println("Rabbitmq connected")
		RabbitMQConn = conn

		Transcript = "transcript"
	})
}
