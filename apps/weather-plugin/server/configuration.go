package main

type configuration struct {
	// No configuration needed for fake weather data
}

func (c *configuration) Clone() *configuration {
	var clone = *c
	return &clone
}

func (c *configuration) IsValid() error {
	return nil
}

