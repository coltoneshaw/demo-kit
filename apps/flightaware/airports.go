package main

import (
	"strings"
)

// AirportCodeMap maps common airport codes to their ICAO codes
var AirportCodeMap = map[string]string{
	// North America
	"ATL": "KATL", // Atlanta Hartsfield-Jackson
	"LAX": "KLAX", // Los Angeles International
	"ORD": "KORD", // Chicago O'Hare
	"DFW": "KDFW", // Dallas/Fort Worth
	"DEN": "KDEN", // Denver International
	"JFK": "KJFK", // New York JFK
	"SFO": "KSFO", // San Francisco
	"SEA": "KSEA", // Seattle-Tacoma
	"LAS": "KLAS", // Las Vegas
	"MCO": "KMCO", // Orlando
	"EWR": "KEWR", // Newark
	"CLT": "KCLT", // Charlotte
	"PHX": "KPHX", // Phoenix
	"IAH": "KIAH", // Houston
	"MIA": "KMIA", // Miami
	"BOS": "KBOS", // Boston
	"MSP": "KMSP", // Minneapolis
	"DTW": "KDTW", // Detroit
	"FLL": "KFLL", // Fort Lauderdale
	"PHL": "KPHL", // Philadelphia
	"LGA": "KLGA", // New York LaGuardia
	"BWI": "KBWI", // Baltimore
	"SLC": "KSLC", // Salt Lake City
	"DCA": "KDCA", // Washington Reagan
	"IAD": "KIAD", // Washington Dulles
	"SAN": "KSAN", // San Diego
	"TPA": "KTPA", // Tampa
	"PDX": "KPDX", // Portland
	"RDU": "KRDU", // Raleigh-Durham
	"AUS": "KAUS", // Austin
	"YYZ": "CYYZ", // Toronto
	"YVR": "CYVR", // Vancouver
	"YUL": "CYUL", // Montreal
	"YYC": "CYYC", // Calgary
	"MEX": "MMMX", // Mexico City

	// Europe
	"LHR": "EGLL", // London Heathrow
	"CDG": "LFPG", // Paris Charles de Gaulle
	"AMS": "EHAM", // Amsterdam
	"FRA": "EDDF", // Frankfurt
	"MAD": "LEMD", // Madrid
	"FCO": "LIRF", // Rome
	"LGW": "EGKK", // London Gatwick
	"MUC": "EDDM", // Munich
	"BCN": "LEBL", // Barcelona
	"ZRH": "LSZH", // Zurich
	"BRU": "EBBR", // Brussels
	"VIE": "LOWW", // Vienna
	"ARN": "ESSA", // Stockholm
	"CPH": "EKCH", // Copenhagen
	"DUB": "EIDW", // Dublin
	"MAN": "EGCC", // Manchester
	"OSL": "ENGM", // Oslo
	"LIS": "LPPT", // Lisbon
	"HEL": "EFHK", // Helsinki
	"ATH": "LGAV", // Athens
	"WAW": "EPWA", // Warsaw
	"PRG": "LKPR", // Prague

	// Asia
	"PEK": "ZBAA", // Beijing
	"HND": "RJTT", // Tokyo Haneda
	"DXB": "OMDB", // Dubai
	"HKG": "VHHH", // Hong Kong
	"ICN": "RKSI", // Seoul Incheon
	"SIN": "WSSS", // Singapore
	"BKK": "VTBS", // Bangkok
	"KUL": "WMKK", // Kuala Lumpur
	"DEL": "VIDP", // Delhi
	"CGK": "WIII", // Jakarta
	"CAN": "ZGGG", // Guangzhou
	"TPE": "RCTP", // Taipei
	"NRT": "RJAA", // Tokyo Narita
	"MNL": "RPLL", // Manila
	"PVG": "ZSPD", // Shanghai Pudong
	"BOM": "VABB", // Mumbai
	"SYD": "YSSY", // Sydney
	"MEL": "YMML", // Melbourne
	"AKL": "NZAA", // Auckland
	"BNE": "YBBN", // Brisbane
}

// GetICAOCode converts a 3-letter airport code to its ICAO code
// If the code is already in ICAO format (4 letters), it returns it unchanged
// If the code is not found in the map, it returns the original code
func GetICAOCode(code string) string {
	// Convert to uppercase
	upperCode := strings.ToUpper(code)
	
	// If it's already 4 letters, assume it's an ICAO code
	if len(upperCode) == 4 {
		return upperCode
	}
	
	// If it's 3 letters, try to convert it
	if len(upperCode) == 3 {
		if icaoCode, exists := AirportCodeMap[upperCode]; exists {
			return icaoCode
		}
	}
	
	// Return the original code if we couldn't convert it
	return upperCode
}
