package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting FlightAware application...")

	// Set up HTTP server
	http.HandleFunc("/flights/departure", handleFlightDepartureRequest)
	http.HandleFunc("/health", handleHealthCheck)
	http.HandleFunc("/webhook", handleIncomingWebhook)

	// Add a debug endpoint to log request details
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Debug request received: %s %s", r.Method, r.URL.Path)
		log.Printf("Headers: %v", r.Header)
		body, _ := io.ReadAll(r.Body)
		log.Printf("Body: %s", string(body))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Debug information logged"))
	})

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
