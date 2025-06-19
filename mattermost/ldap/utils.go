package ldap

// GetStringFromMap safely extracts a string value from a map
func GetStringFromMap(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

// GetStringFromInterface safely extracts a string value from an interface
func GetStringFromInterface(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}