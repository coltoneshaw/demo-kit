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

	// Create subscription manager
	subscriptionsFile := "./flight_subscriptions.json"
	subscriptionManager := NewSubscriptionManager(subscriptionsFile)
	log.Printf("Loaded %d subscriptions", len(subscriptionManager.Subscriptions))

	// Restart existing subscriptions
	for _, sub := range subscriptionManager.Subscriptions {
		sub.StopChan = make(chan struct{})
		go startFlightSubscription(sub, subscriptionManager)
	}

	// Set up graceful shutdown
	setupGracefulShutdown(subscriptionManager)

	// Set up HTTP server
	http.HandleFunc("/flights/departure", handleFlightDepartureRequest)
	http.HandleFunc("/health", handleHealthCheck)
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleIncomingWebhook(w, r, subscriptionManager)
	})

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

// setupGracefulShutdown sets up graceful shutdown for the subscription manager
func setupGracefulShutdown(sm *SubscriptionManager) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Received shutdown signal, saving subscriptions...")
		if err := sm.SaveToFile(); err != nil {
			log.Printf("Error saving subscriptions: %v", err)
		} else {
			log.Println("Subscriptions saved successfully")
		}
		os.Exit(0)
	}()
}

// startFlightSubscription starts a subscription to flight departures
func startFlightSubscription(sub *FlightSubscription, sm *SubscriptionManager) {
	log.Printf("Starting subscription %s for airport %s", sub.ID, sub.Airport)

	// Set up ticker for periodic updates
	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Second)
	defer ticker.Stop()

	// Function to fetch and send flight data
	fetchAndSendFlights := func() {
		// Get current time
		now := time.Now()
		// Get time 24 hours ago
		past := now.Add(-24 * time.Hour)

		// Get flight data
		flights, err := getDepartureFlights(sub.Airport, past.Unix(), now.Unix())
		if err != nil {
			log.Printf("Error fetching flight data for subscription %s: %v", sub.ID, err)
			return
		}

		// Format the response
		response := formatFlightResponse(flights, sub.Airport, past.Unix(), now.Unix())

		// Send to Mattermost
		err = sendMattermostMessage(sub.ChannelID, response)
		if err != nil {
			log.Printf("Error sending message to Mattermost for subscription %s: %v", sub.ID, err)
			return
		}

		// Update last updated time
		sub.LastUpdated = now
		sm.SaveToFile()
	}

	// Fetch and send initial data
	fetchAndSendFlights()

	// Wait for updates or cancellation
	for {
		select {
		case <-ticker.C:
			fetchAndSendFlights()
		case <-sub.StopChan:
			log.Printf("Stopping subscription %s", sub.ID)
			return
		}
	}
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
