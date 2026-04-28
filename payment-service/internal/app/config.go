package app

import "os"

type Config struct {
	HTTPAddr    string
	GRPCAddr    string
	DBDSN       string
	RabbitMQURL string
}

func LoadConfig() Config {
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8081"
	}

	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":50051"
	}

	return Config{
		HTTPAddr:    httpAddr,
		GRPCAddr:    grpcAddr,
		DBDSN:       os.Getenv("DB_DSN"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
	}
}
