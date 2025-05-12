package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/pkg/errors"
)

func main() {
	http.HandleFunc("/weather", weatherHandler)
	http.HandleFunc("/incoming", mattermostHandler)

	fmt.Println("Listening on http://localhost:8085")
	err := http.ListenAndServe(":8085", nil)
	if err != nil {
		fmt.Println("Failed to start server:", err)
		os.Exit(1)
	}
}

func getAPIKey() (apiKey string, err error) {
	apiKey = os.Getenv("TOMORROW_API_KEY")
	if apiKey == "" {
		return "", errors.New("API key not set in TOMORROW_API_KEY")
	}

	return apiKey, nil
}

func weatherHandler(w http.ResponseWriter, r *http.Request) {

	location := r.URL.Query().Get("location")
	if location == "" {
		http.Error(w, "Missing 'location' query parameter", http.StatusBadRequest)
		return
	}

	escapedLocation := url.QueryEscape(location)
	apiURL := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", escapedLocation, apiKey)
	resp, err := http.Get(apiURL)
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("API request failed: %s", resp.Status), resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		http.Error(w, "Failed to parse weather response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jsonResponse)
}

func mattermostHandler(w http.ResponseWriter, r *http.Request) {
	apiKey := os.Getenv("TOMORROW_API_KEY")
	if apiKey == "" {
		http.Error(w, "API key not set in TOMORROW_API_KEY", http.StatusInternalServerError)
		return
	}

	// We'll ignore the actual POST payload for now
	location := "wendell,nc"
	escapedLocation := url.QueryEscape(location)
	apiURL := fmt.Sprintf("https://api.tomorrow.io/v4/weather/realtime?location=%s&apikey=%s", escapedLocation, apiKey)

	resp, err := http.Get(apiURL)
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("API request failed: %s", resp.Status), resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		http.Error(w, "Failed to parse weather response", http.StatusInternalServerError)
		return
	}

	// Extract data
	values := jsonResponse["data"].(map[string]interface{})["values"].(map[string]interface{})
	temp := values["temperature"]
	code := int(values["weatherCode"].(float64))

	condition := mapWeatherCode(code)

	// Respond with a plaintext message for Mattermost
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "🌤️ Weather in Wendell, NC: %.1f°C and %s", temp, condition)
}

func mapWeatherCode(code int) string {
	switch code {
	case 1000:
		return "clear"
	case 1100:
		return "mostly clear"
	case 1101:
		return "partly cloudy"
	case 1102:
		return "mostly cloudy"
	case 4000:
		return "raining"
	default:
		return "unknown conditions"
	}
}
