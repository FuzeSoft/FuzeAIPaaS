package main

import (
	"log"

	"fuze-ai-paas/backend/internal/bootstrap"
)

func main() {
	srv, err := bootstrap.NewServer(bootstrap.LoadConfig())
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}
	if err := srv.Run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}