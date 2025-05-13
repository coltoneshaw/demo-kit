package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// getDepartureFlights fetches flights departing from the specified airport
func getDepartureFlights(airport string, start, end int64) (*DepartureFlights, error) {
	// Construct the API URL
	apiURL := fmt.Sprintf("https://opensky-network.org/api/flights/departure?airport=%s&begin=%d&end=%d",
		strings.ToUpper(airport), start, end)

	log.Printf("Requesting flights from: %s", apiURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make the request
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("error making request to OpenSky API: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenSky API returned non-OK status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var flights []Flight
	if err := json.NewDecoder(resp.Body).Decode(&flights); err != nil {
		return nil, fmt.Errorf("error parsing OpenSky API response: %v", err)
	}

	// Create the result
	result := &DepartureFlights{
		Airport: airport,
		Start:   start,
		End:     end,
		Flights: flights,
	}

	return result, nil
}

// formatFlightResponse formats flight data into a readable markdown table
func formatFlightResponse(flights *DepartureFlights, airport string, start, end int64) string {
	if len(flights.Flights) == 0 {
		return fmt.Sprintf("No departures found from %s in the specified time range.", airport)
	}

	// Format the time range
	startTime := time.Unix(start, 0).Format(time.RFC1123)
	endTime := time.Unix(end, 0).Format(time.RFC1123)

	// Build the response
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Departures from %s**\n", airport))
	sb.WriteString(fmt.Sprintf("Time range: %s to %s\n\n", startTime, endTime))

	// Create markdown table header
	sb.WriteString("| Flight | Airline | Departure Time | Destination | Duration |\n")
	sb.WriteString("|--------|---------|---------------|-------------|----------|\n")

	// Limit to 20 flights to avoid message size limits
	maxFlights := 20
	if len(flights.Flights) < maxFlights {
		maxFlights = len(flights.Flights)
	}

	for i := 0; i < maxFlights; i++ {
		flight := flights.Flights[i]
		departureTime := time.Unix(flight.FirstSeen, 0).Format("15:04 MST")

		// Clean up callsign (remove trailing spaces)
		callsign := strings.TrimSpace(flight.Callsign)

		// Get destination if available
		destination := "-"
		if flight.EstArrivalAirport != "" {
			destinationCode := flight.EstArrivalAirport
			// Try to convert ICAO code to more recognizable 3-letter code if possible
			for code, icao := range AirportCodeMap {
				if icao == flight.EstArrivalAirport {
					destinationCode = code
					break
				}
			}
			destination = destinationCode
		}

		// Get flight duration if available
		duration := "-"
		if flight.LastSeen > flight.FirstSeen {
			durationMinutes := (flight.LastSeen - flight.FirstSeen) / 60
			duration = fmt.Sprintf("%d min", durationMinutes)
		}

		// Get airline information
		airlineName, _ := GetAirlineInfo(callsign)
		if airlineName == "" {
			airlineName = "Unknown"
		}

		// Add row to table
		sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %s |\n",
			callsign, airlineName, departureTime, destination, duration))
	}

	if len(flights.Flights) > maxFlights {
		sb.WriteString(fmt.Sprintf("\n_Showing %d of %d total flights_", maxFlights, len(flights.Flights)))
	}

	return sb.String()
}
