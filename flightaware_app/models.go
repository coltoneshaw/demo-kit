package main

// OpenSkyResponse represents the response from the OpenSky API
type OpenSkyResponse struct {
	Time   int64        `json:"time"`
	States [][]interface{} `json:"states"`
}

// Flight represents a processed flight from OpenSky data
type Flight struct {
	Icao24        string  `json:"icao24"`             // ICAO 24-bit address of the transponder
	Callsign      string  `json:"callsign"`           // Callsign of the vehicle
	Origin        string  `json:"origin"`             // Origin airport ICAO code
	Destination   string  `json:"estArrivalAirport"`  // Destination airport ICAO code
	TimePosition  int64   `json:"timePosition"`  // Unix timestamp for the last position update
	LastContact   int64   `json:"lastContact"`   // Unix timestamp for the last update in general
	Longitude     float64 `json:"longitude"`     // WGS-84 longitude in decimal degrees
	Latitude      float64 `json:"latitude"`      // WGS-84 latitude in decimal degrees
	BaroAltitude  float64 `json:"baroAltitude"`  // Barometric altitude in meters
	OnGround      bool    `json:"onGround"`      // Indicates if the position was retrieved from a surface position report
	Velocity      float64 `json:"velocity"`      // Velocity over ground in m/s
	TrueTrack     float64 `json:"trueTrack"`     // True track in decimal degrees (0 is north)
	VerticalRate  float64 `json:"verticalRate"`  // Vertical rate in m/s
	Sensors       []int   `json:"sensors"`       // IDs of the receivers which contributed to this state vector
	GeoAltitude   float64 `json:"geoAltitude"`   // Geometric altitude in meters
	Squawk        string  `json:"squawk"`        // The transponder code
	Spi           bool    `json:"spi"`           // Whether flight status indicates special purpose indicator
	PositionSource int    `json:"positionSource"` // Origin of this state's position
}

// DepartureFlights represents a collection of flights departing from an airport
type DepartureFlights struct {
	Airport  string   `json:"airport"`
	Start    int64    `json:"start"`
	End      int64    `json:"end"`
	Flights  []Flight `json:"flights"`
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
