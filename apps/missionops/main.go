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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Mission Operations application...")

	// Initialize the Mattermost client as early as possible to validate connection
	mmClient, err := NewClient()
	if err != nil {
		log.Fatalf("Failed to initialize Mattermost client: %v", err)
	}

	// Create mission manager with file path in the mounted volume
	missionsFile := "/app/data/missions.json"
	log.Printf("Using missions file: %s", missionsFile)
	missionManager := NewMissionManager(missionsFile)

	// Create subscription manager with file path in the mounted volume
	subscriptionsFile := "/app/data/mission_subscriptions.json"
	log.Printf("Using subscriptions file: %s", subscriptionsFile)
	subscriptionManager := NewSubscriptionManager(subscriptionsFile)

	// Restart existing subscriptions
	restartSubscriptions(subscriptionManager, missionManager, mmClient)

	// Set up graceful shutdown
	setupGracefulShutdown(missionManager, subscriptionManager)

	// Set up HTTP server
	http.HandleFunc("/health", handleHealthCheck)
	
	// Set up webhook handler
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleIncomingWebhook(w, r, missionManager, subscriptionManager)
	})

	// Add a debug endpoint to log request details
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Debug request received: %s %s", r.Method, r.URL.Path)
		log.Printf("Headers: %v", r.Header)
		body := r.FormValue("text")
		log.Printf("Body: %s", body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Debug information logged"))
	})

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8087" // Default port
	}

	// Set up graceful shutdown for HTTP server
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

// setupGracefulShutdown sets up graceful shutdown for the managers
func setupGracefulShutdown(mm *MissionManager, sm *SubscriptionManager) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Received shutdown signal, saving data...")
		
		// Save missions
		if err := mm.SaveToFile(); err != nil {
			log.Printf("Error saving missions: %v", err)
		} else {
			log.Println("Missions saved successfully")
		}
		
		// Save subscriptions
		if err := sm.SaveToFile(); err != nil {
			log.Printf("Error saving subscriptions: %v", err)
		} else {
			log.Println("Subscriptions saved successfully")
		}
		
		os.Exit(0)
	}()
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}