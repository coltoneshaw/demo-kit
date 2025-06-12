package flight

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Flight struct {
	Icao24                        string `json:"icao24"`
	FirstSeen                     int64  `json:"firstSeen"`
	EstDepartureAirport           string `json:"estDepartureAirport"`
	LastSeen                      int64  `json:"lastSeen"`
	EstArrivalAirport             string `json:"estArrivalAirport"`
	Callsign                      string `json:"callsign"`
	EstDepartureAirportHorizDistance int  `json:"estDepartureAirportHorizDistance"`
	EstDepartureAirportVertDistance   int  `json:"estDepartureAirportVertDistance"`
	EstArrivalAirportHorizDistance   int  `json:"estArrivalAirportHorizDistance"`
	EstArrivalAirportVertDistance     int  `json:"estArrivalAirportVertDistance"`
	DepartureAirportCandidatesCount  int  `json:"departureAirportCandidatesCount"`
	ArrivalAirportCandidatesCount    int  `json:"arrivalAirportCandidatesCount"`
}

type DepartureFlights struct {
	Airport string   `json:"airport"`
	Start   int64    `json:"start"`
	End     int64    `json:"end"`
	Flights []Flight `json:"flights"`
}

type FlightInterface interface {
	GetDepartureFlights(airport string) (*DepartureFlights, error)
	FormatFlightResponse(flights *DepartureFlights, airport string) string
}

type FlightService struct {
	bundlePath string
	flights    []Flight
}

func NewFlightService(bundlePath string) (FlightInterface, error) {
	fs := &FlightService{
		bundlePath: bundlePath,
	}
	if err := fs.loadFlights(); err != nil {
		return nil, fmt.Errorf("failed to initialize flight service: %w", err)
	}
	return fs, nil
}

func (fs *FlightService) loadFlights() error {
	flightsPath := filepath.Join(fs.bundlePath, "assets", "flights.json")
	data, err := os.ReadFile(flightsPath)
	if err != nil {
		return fmt.Errorf("error reading flights.json: %v", err)
	}

	if err := json.Unmarshal(data, &fs.flights); err != nil {
		return fmt.Errorf("error parsing flights.json: %v", err)
	}

	return nil
}

func (fs *FlightService) GetDepartureFlights(airport string) (*DepartureFlights, error) {
	// Convert airport code to ICAO format if needed
	icaoAirport := fs.getICAOCode(airport)
	
	// Use current time and 6 hours ago as default time range for realistic timestamps
	end := time.Now().Unix()
	start := time.Now().Add(-6 * time.Hour).Unix()
	
	// Generate random flights for any airport
	randomFlights := fs.generateRandomFlights(icaoAirport, start, end)

	result := &DepartureFlights{
		Airport: icaoAirport,
		Start:   start,
		End:     end,
		Flights: randomFlights,
	}

	return result, nil
}

func (fs *FlightService) generateRandomFlights(airport string, start, end int64) []Flight {
	if len(fs.flights) == 0 {
		return []Flight{}
	}

	// Generate random number of flights (between 3 and 8)
	numFlights := rand.Intn(6) + 3
	if numFlights > len(fs.flights) {
		numFlights = len(fs.flights)
	}

	// Create a copy of flights and shuffle them
	availableFlights := make([]Flight, len(fs.flights))
	copy(availableFlights, fs.flights)
	
	// Shuffle the flights
	for i := range availableFlights {
		j := rand.Intn(i + 1)
		availableFlights[i], availableFlights[j] = availableFlights[j], availableFlights[i]
	}

	// Select random flights and modify them for the requested airport
	var randomFlights []Flight
	timeRange := end - start
	
	for i := 0; i < numFlights; i++ {
		flight := availableFlights[i]
		
		// Modify the flight to appear as if it's departing from the requested airport
		flight.EstDepartureAirport = airport
		
		// Generate random departure time within the requested time range
		randomOffset := rand.Int63n(timeRange)
		flight.FirstSeen = start + randomOffset
		
		// Set arrival time (flight duration between 1-8 hours)
		flightDuration := rand.Int63n(7*3600) + 3600 // 1-8 hours in seconds
		flight.LastSeen = flight.FirstSeen + flightDuration
		
		randomFlights = append(randomFlights, flight)
	}

	return randomFlights
}

func (fs *FlightService) FormatFlightResponse(flights *DepartureFlights, airport string) string {
	if len(flights.Flights) == 0 {
		return fmt.Sprintf("No departures found from %s.", airport)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Recent Departures from %s**\n\n", airport))

	sb.WriteString("| Flight | Airline | Departure Time | Destination | Duration |\n")
	sb.WriteString("|--------|---------|---------------|-------------|----------|\n")

	maxFlights := 20
	if len(flights.Flights) < maxFlights {
		maxFlights = len(flights.Flights)
	}

	for i := 0; i < maxFlights; i++ {
		flight := flights.Flights[i]
		departureTime := time.Unix(flight.FirstSeen, 0).Format("15:04 MST")

		callsign := strings.TrimSpace(flight.Callsign)

		destination := "-"
		if flight.EstArrivalAirport != "" {
			destination = flight.EstArrivalAirport
		}

		duration := "-"
		if flight.LastSeen > flight.FirstSeen {
			durationMinutes := (flight.LastSeen - flight.FirstSeen) / 60
			duration = fmt.Sprintf("%d min", durationMinutes)
		}

		airlineName := fs.getAirlineInfo(callsign)
		if airlineName == "" {
			airlineName = "Unknown"
		}

		sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %s |\n",
			callsign, airlineName, departureTime, destination, duration))
	}

	if len(flights.Flights) > maxFlights {
		sb.WriteString(fmt.Sprintf("\n_Showing %d of %d total flights_", maxFlights, len(flights.Flights)))
	}

	return sb.String()
}

func (fs *FlightService) getICAOCode(airport string) string {
	// Simple mapping for common airports
	airportMap := map[string]string{
		"SFO": "KSFO",
		"LAX": "KLAX", 
		"JFK": "KJFK",
		"ORD": "KORD",
		"DFW": "KDFW",
		"LAS": "KLAS",
		"BOS": "KBOS",
		"DEN": "KDEN",
		"LHR": "EGLL",
		"RDU": "KRDU",
	}

	if icao, exists := airportMap[strings.ToUpper(airport)]; exists {
		return icao
	}
	return strings.ToUpper(airport)
}

func (fs *FlightService) getAirlineInfo(callsign string) string {
	// Simple airline mapping based on callsign prefixes
	airlineMap := map[string]string{
		"UAL": "United Airlines",
		"DL":  "Delta Air Lines", 
		"AA":  "American Airlines",
		"SW":  "Southwest Airlines",
		"JB":  "JetBlue Airways",
		"B6":  "JetBlue Airways",
		"VS":  "Virgin Atlantic",
		"F9":  "Frontier Airlines",
		"BA":  "British Airways",
	}

	for prefix, airline := range airlineMap {
		if strings.HasPrefix(strings.ToUpper(callsign), prefix) {
			return airline
		}
	}
	return "Unknown"
}