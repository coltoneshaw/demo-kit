package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		log.Println("Warning: WEATHER_API_KEY environment variable not set, using default")
		apiKey = "c5AeEo7A30nZmTHZkCs0fQXT8JcUFWJC" // Fallback to default if not set
	}
	log.Printf("Using API key: %s", apiKey)

	// Create subscription manager with file path
	subscriptionsFile := "./subscriptions.json"
	subscriptionManager := NewSubscriptionManager(subscriptionsFile)
	log.Printf("Using subscriptions file: %s", subscriptionsFile)

	// Set up HTTP server
	http.HandleFunc("/health", handleHealthCheck)

	http.HandleFunc("/api-usage", func(w http.ResponseWriter, r *http.Request) {
		handleAPIUsage(w, r, subscriptionManager)
	})

	http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		handleWeatherRequest(w, r, apiKey)
	})

	http.HandleFunc("/incoming", func(w http.ResponseWriter, r *http.Request) {
		handleIncomingWebhook(w, r, apiKey, subscriptionManager)
	})

	// Create a directory for static files if it doesn't exist
	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		if err := os.MkdirAll(staticDir, 0755); err != nil {
			log.Fatalf("Failed to create static directory: %v", err)
		}
	}

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Special handler for bot.png
	http.HandleFunc("/bot.png", func(w http.ResponseWriter, r *http.Request) {
		handleBotImage(w, r, staticDir)
	})

	// Start active subscriptions
	log.Printf("Starting %d saved subscriptions...", len(subscriptionManager.Subscriptions))
	for _, sub := range subscriptionManager.Subscriptions {
		// Initialize the stop channel
		sub.StopChan = make(chan struct{})

		// Start the subscription in a goroutine
		go startSubscription(sub, apiKey, subscriptionManager)
	}

	// Set up graceful shutdown to save subscriptions
	setupGracefulShutdown(subscriptionManager)

	// Start the server
	port := "8085"
	serverURL := fmt.Sprintf("http://0.0.0.0:%s", port)
	log.Printf("Server starting on port %s (listening on all interfaces)...", port)
	log.Printf("Bot image will be available at %s/bot.png", serverURL)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// setupGracefulShutdown sets up signal handling for graceful shutdown
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
