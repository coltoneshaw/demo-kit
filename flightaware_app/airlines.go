package main

import (
	"strings"
)

// AirlineInfo stores information about an airline
type AirlineInfo struct {
	Name     string
	Country  string
	IATACode string
}

// AirlineMap maps airline ICAO codes to airline information
var AirlineMap = map[string]AirlineInfo{
	// Major US Airlines
	"AAL": {Name: "American Airlines", Country: "United States", IATACode: "AA"},
	"UAL": {Name: "United Airlines", Country: "United States", IATACode: "UA"},
	"DAL": {Name: "Delta Air Lines", Country: "United States", IATACode: "DL"},
	"SWA": {Name: "Southwest Airlines", Country: "United States", IATACode: "WN"},
	"JBU": {Name: "JetBlue Airways", Country: "United States", IATACode: "B6"},
	"ASA": {Name: "Alaska Airlines", Country: "United States", IATACode: "AS"},
	"FFT": {Name: "Frontier Airlines", Country: "United States", IATACode: "F9"},
	"SKW": {Name: "SkyWest Airlines", Country: "United States", IATACode: "OO"},
	"NKS": {Name: "Spirit Airlines", Country: "United States", IATACode: "NK"},
	"HAL": {Name: "Hawaiian Airlines", Country: "United States", IATACode: "HA"},

	// Major European Airlines
	"DLH": {Name: "Lufthansa", Country: "Germany", IATACode: "LH"},
	"AFR": {Name: "Air France", Country: "France", IATACode: "AF"},
	"BAW": {Name: "British Airways", Country: "United Kingdom", IATACode: "BA"},
	"KLM": {Name: "KLM Royal Dutch Airlines", Country: "Netherlands", IATACode: "KL"},
	"IBE": {Name: "Iberia", Country: "Spain", IATACode: "IB"},
	"SAS": {Name: "Scandinavian Airlines", Country: "Sweden", IATACode: "SK"},
	"SWR": {Name: "Swiss International Air Lines", Country: "Switzerland", IATACode: "LX"},
	"AZA": {Name: "Alitalia", Country: "Italy", IATACode: "AZ"},
	"VIR": {Name: "Virgin Atlantic", Country: "United Kingdom", IATACode: "VS"},
	"EZY": {Name: "easyJet", Country: "United Kingdom", IATACode: "U2"},
	"RYR": {Name: "Ryanair", Country: "Ireland", IATACode: "FR"},
	"WZZ": {Name: "Wizz Air", Country: "Hungary", IATACode: "W6"},
	"NLY": {Name: "Air UK", Country: "United Kingdom", IATACode: "UK"},

	// Major Asian Airlines
	"CPA": {Name: "Cathay Pacific", Country: "Hong Kong", IATACode: "CX"},
	"CCA": {Name: "Air China", Country: "China", IATACode: "CA"},
	"CSN": {Name: "China Southern Airlines", Country: "China", IATACode: "CZ"},
	"CES": {Name: "China Eastern Airlines", Country: "China", IATACode: "MU"},
	"JAL": {Name: "Japan Airlines", Country: "Japan", IATACode: "JL"},
	"ANA": {Name: "All Nippon Airways", Country: "Japan", IATACode: "NH"},
	"KAL": {Name: "Korean Air", Country: "South Korea", IATACode: "KE"},
	"AAR": {Name: "Asiana Airlines", Country: "South Korea", IATACode: "OZ"},
	"SIA": {Name: "Singapore Airlines", Country: "Singapore", IATACode: "SQ"},
	"THY": {Name: "Turkish Airlines", Country: "Turkey", IATACode: "TK"},
	"UAE": {Name: "Emirates", Country: "United Arab Emirates", IATACode: "EK"},
	"ETD": {Name: "Etihad Airways", Country: "United Arab Emirates", IATACode: "EY"},
	"QTR": {Name: "Qatar Airways", Country: "Qatar", IATACode: "QR"},

	// Major Oceania Airlines
	"QFA": {Name: "Qantas", Country: "Australia", IATACode: "QF"},
	"JST": {Name: "Jetstar Airways", Country: "Australia", IATACode: "JQ"},
	"ANZ": {Name: "Air New Zealand", Country: "New Zealand", IATACode: "NZ"},
}

// AircraftTypeMap maps aircraft type codes to their full names
var AircraftTypeMap = map[string]string{
	// Airbus
	"A319": "Airbus A319",
	"A320": "Airbus A320",
	"A321": "Airbus A321",
	"A332": "Airbus A330-200",
	"A333": "Airbus A330-300",
	"A339": "Airbus A330-900",
	"A343": "Airbus A340-300",
	"A346": "Airbus A340-600",
	"A359": "Airbus A350-900",
	"A35K": "Airbus A350-1000",
	"A388": "Airbus A380-800",

	// Boeing
	"B737": "Boeing 737",
	"B738": "Boeing 737-800",
	"B739": "Boeing 737-900",
	"B744": "Boeing 747-400",
	"B748": "Boeing 747-8",
	"B752": "Boeing 757-200",
	"B753": "Boeing 757-300",
	"B762": "Boeing 767-200",
	"B763": "Boeing 767-300",
	"B764": "Boeing 767-400",
	"B772": "Boeing 777-200",
	"B77L": "Boeing 777-200LR",
	"B773": "Boeing 777-300",
	"B77W": "Boeing 777-300ER",
	"B788": "Boeing 787-8",
	"B789": "Boeing 787-9",
	"B78X": "Boeing 787-10",

	// Embraer
	"E170": "Embraer E170",
	"E175": "Embraer E175",
	"E190": "Embraer E190",
	"E195": "Embraer E195",

	// Bombardier
	"CRJ2": "Bombardier CRJ-200",
	"CRJ7": "Bombardier CRJ-700",
	"CRJ9": "Bombardier CRJ-900",
	"CRJX": "Bombardier CRJ-1000",
	"DH8D": "Bombardier Dash 8 Q400",
}

// GetAirlineInfo extracts airline information from a callsign
func GetAirlineInfo(callsign string) (string, string) {
	// Clean up callsign
	callsign = strings.TrimSpace(callsign)

	// Most airline callsigns follow the pattern of 3-letter ICAO code followed by numbers
	// For example: "DLH123" for Lufthansa flight 123
	if len(callsign) >= 3 {
		airlineCode := callsign[0:3]
		if airline, exists := AirlineMap[airlineCode]; exists {
			return airline.Name, airline.Country
		}
	}

	return "", ""
}

// GetAircraftType tries to extract aircraft type from a callsign
// This is a simplification as aircraft type is not typically encoded in callsigns
// In a real system, you would need to query additional flight data
func GetAircraftType(callsign string) string {
	// This is a placeholder - in reality, you would need to query a flight database
	// to get the actual aircraft type for a specific flight

	// For demonstration purposes, we'll return a few common types based on airline
	if len(callsign) >= 3 {
		airlineCode := callsign[0:3]

		// Just some examples based on common fleet types
		switch airlineCode {
		case "DLH": // Lufthansa
			return "Airbus A320 or Boeing 747"
		case "BAW": // British Airways
			return "Airbus A320 or Boeing 777"
		case "AAL": // American Airlines
			return "Boeing 737 or Boeing 787"
		case "UAL": // United Airlines
			return "Boeing 737 or Boeing 777"
		case "DAL": // Delta
			return "Airbus A320 or Boeing 767"
		case "SWA": // Southwest
			return "Boeing 737"
		case "RYR": // Ryanair
			return "Boeing 737"
		case "EZY": // easyJet
			return "Airbus A320"
		case "UAE": // Emirates
			return "Boeing 777 or Airbus A380"
		case "QFA": // Qantas
			return "Boeing 737 or Airbus A330"
		}
	}

	return "Unknown"
}
