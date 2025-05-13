package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Set up logging
	log.Println("Starting FlightAware application...")

	// Set up HTTP server
	http.HandleFunc("/flights/departure", handleFlightDepartureRequest)
	http.HandleFunc("/health", handleHealthCheck)
	http.HandleFunc("/webhook", handleIncomingWebhook)

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086" // Default port
	}

	// Set up graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
