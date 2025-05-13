package main

// OpenSkyResponse represents the response from the OpenSky API
type OpenSkyResponse struct {
	Time   int64        `json:"time"`
	States [][]interface{} `json:"states"`
}

// Flight represents a processed flight from OpenSky data
type Flight struct {
	Icao24                        string `json:"icao24"`                        // ICAO 24-bit address of the transponder
	FirstSeen                     int64  `json:"firstSeen"`                     // Time of first message received
	EstDepartureAirport           string `json:"estDepartureAirport"`           // ICAO code of the estimated departure airport
	LastSeen                      int64  `json:"lastSeen"`                      // Time of last message received
	EstArrivalAirport             string `json:"estArrivalAirport"`             // ICAO code of the estimated arrival airport
	Callsign                      string `json:"callsign"`                      // Callsign of the vehicle
	EstDepartureAirportHorizDistance int  `json:"estDepartureAirportHorizDistance"` // Horizontal distance from departure airport
	EstDepartureAirportVertDistance   int  `json:"estDepartureAirportVertDistance"`   // Vertical distance from departure airport
	EstArrivalAirportHorizDistance   int  `json:"estArrivalAirportHorizDistance"`   // Horizontal distance from arrival airport
	EstArrivalAirportVertDistance     int  `json:"estArrivalAirportVertDistance"`     // Vertical distance from arrival airport
	DepartureAirportCandidatesCount  int  `json:"departureAirportCandidatesCount"`  // Number of other possible departure airports
	ArrivalAirportCandidatesCount    int  `json:"arrivalAirportCandidatesCount"`    // Number of other possible arrival airports
}

// DepartureFlights represents a collection of flights departing from an airport
type DepartureFlights struct {
	Airport  string   `json:"airport"`
	Start    int64    `json:"start"`
	End      int64    `json:"end"`
	Flights  []Flight `json:"flights"`
}

// EnhancedFlight adds derived information to a flight
type EnhancedFlight struct {
	Flight       Flight
	Airline      string
	Country      string
	AircraftType string
}

// MattermostPayload represents the incoming webhook payload from Mattermost
type MattermostPayload struct {
	Text       string `json:"text"`
	UserID     string `json:"user_id"`
	Channel    string `json:"channel_name"`
	ChannelID  string `json:"channel_id"`
	Command    string `json:"command"`
	TeamDomain string `json:"team_domain"`
	Token      string `json:"token"`
}

// MattermostResponse represents the response to send back to Mattermost
type MattermostResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"response_type"`
	ChannelID    string `json:"channel_id,omitempty"`
}
