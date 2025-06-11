package main

import (
	"reflect"

	"github.com/pkg/errors"
)

type configuration struct {
	TomorrowAPIKey string `json:"tomorrow_api_key"`
}

func (c *configuration) Clone() *configuration {
	var clone = *c
	return &clone
}

func (c *configuration) IsValid() error {
	if c.TomorrowAPIKey == "" {
		return errors.New("tomorrow.io API key is required")
	}
	return nil
}

func (c *configuration) getDisplayName() string {
	return "Weather Plugin Configuration"
}

func (c *configuration) setDefaults() {
	// No defaults to set
}

func (c *configuration) isEqual(other *configuration) bool {
	if other == nil {
		return false
	}
	return reflect.DeepEqual(c, other)
}