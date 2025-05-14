package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// startSubscription starts a subscription in a goroutine
func startSubscription(sub *Subscription, apiKey string, subscriptionManager *SubscriptionManager) {
	// Get initial weather data
	weatherData, err := getWeatherData(sub.Location, apiKey)
	if err != nil {
		// Log the error
		log.Printf("Error fetching initial weather data for subscription %s: %v", sub.ID, err)
		
		// Send error message to the channel
		errorMsg := fmt.Sprintf("⚠️ Could not fetch weather data for subscription to **%s** (ID: `%s`): %v", 
			sub.Location, sub.ID, err)
		sendMattermostMessage(sub.ChannelID, errorMsg)
		
		// Don't return - continue with subscription setup even if initial data fetch fails
		// This allows subscriptions to recover if the API issue is temporary
	} else {
		// Only send the initial update if we successfully got data
		weatherText := formatWeatherResponse(weatherData)
		sendMattermostMessage(sub.ChannelID, weatherText)
	}

	// Create a ticker for periodic updates (convert milliseconds to duration)
	ticker := time.NewTicker(time.Duration(sub.UpdateFrequency) * time.Millisecond)
	defer ticker.Stop()
	
	// Track consecutive failures
	consecutiveFailures := 0
	maxConsecutiveFailures := 5

	for {
		select {
		case <-ticker.C:
			// Get updated weather data
			weatherData, err := getWeatherData(sub.Location, apiKey)
			if err != nil {
				consecutiveFailures++
				log.Printf("Error fetching weather data for subscription %s: %v (Failure #%d)", 
					sub.ID, err, consecutiveFailures)
				
				// Only notify about errors occasionally to avoid spam
				if consecutiveFailures == 1 || consecutiveFailures == maxConsecutiveFailures {
					errorMsg := fmt.Sprintf("⚠️ Error updating weather for **%s**: %v", sub.Location, err)
					sendMattermostMessage(sub.ChannelID, errorMsg)
				}
				
				// If we've had too many consecutive failures, increase the update interval temporarily
				if consecutiveFailures >= maxConsecutiveFailures {
					log.Printf("Too many consecutive failures for subscription %s, backing off", sub.ID)
					// Double the ticker interval temporarily
					ticker.Reset(time.Duration(sub.UpdateFrequency*2) * time.Millisecond)
				}
				
				continue
			}
			
			// Reset failure counter on success
			if consecutiveFailures > 0 {
				log.Printf("Successfully recovered subscription %s after %d failures", sub.ID, consecutiveFailures)
				consecutiveFailures = 0
				// Reset ticker to normal interval
				ticker.Reset(time.Duration(sub.UpdateFrequency) * time.Millisecond)
			}

			// Send weather update
			weatherText := formatWeatherResponse(weatherData)
			sendMattermostMessage(sub.ChannelID, weatherText)

			// Update last updated time
			sub.LastUpdated = time.Now()

			// Only save to file occasionally (every 6 hours) to reduce disk I/O
			if time.Since(sub.LastUpdated).Hours() >= 6 {
				subscriptionManager.SaveToFile()
			}
		case <-sub.StopChan:
			log.Printf("Stopping subscription %s", sub.ID)
			return
		}
	}
}

func main() {
	// Get API key from environment variable
	apiKey := os.Getenv("TOMORROW_API_KEY")
	if apiKey == "" {
		log.Fatal("Error: TOMORROW_API_KEY environment variable not set. Please set this environment variable in the env_vars.env file.")
	}
	log.Println("Tomorrow.io API key loaded successfully")

	// Create subscription manager with file path in the mounted volume
	subscriptionsFile := "./data/subscriptions.json"
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

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
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
