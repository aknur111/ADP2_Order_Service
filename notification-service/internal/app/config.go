package app

import "os"

type Config struct {
	DBDSN       string
	RabbitMQURL string
}

func LoadConfig() Config {
	return Config{
		DBDSN:       os.Getenv("DB_DSN"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
	}
}
