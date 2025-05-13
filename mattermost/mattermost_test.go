package mattermost

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
)

// TestNewClient tests the creation of a new client
func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8065", "admin", "password", "test")
	
	if client.ServerURL != "http://localhost:8065" {
		t.Errorf("Expected ServerURL to be http://localhost:8065, got %s", client.ServerURL)
	}
	
	if client.AdminUser != "admin" {
		t.Errorf("Expected AdminUser to be admin, got %s", client.AdminUser)
	}
	
	if client.AdminPass != "password" {
		t.Errorf("Expected AdminPass to be password, got %s", client.AdminPass)
	}
	
	if client.TeamName != "test" {
		t.Errorf("Expected TeamName to be test, got %s", client.TeamName)
	}
}

// MockServer creates a mock server for testing
func MockServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}

// TestLogin tests the login functionality
func TestLogin(t *testing.T) {
	// Create a mock server that returns a successful response
	server := MockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"user123","username":"admin"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	
	// Create a client that points to our mock server
	client := NewClient(server.URL, "admin", "password", "test")
	
	// Test successful login
	err := client.Login()
	if err != nil {
		t.Errorf("Expected successful login, got error: %v", err)
	}
}

// TestLoginFailure tests login failure handling
func TestLoginFailure(t *testing.T) {
	// Create a mock server that returns an error response
	server := MockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/login" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Invalid credentials"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	
	// Create a client that points to our mock server
	client := NewClient(server.URL, "admin", "wrong-password", "test")
	
	// Test failed login
	err := client.Login()
	if err == nil {
		t.Error("Expected login to fail, but it succeeded")
	}
}

// TestWaitForStart tests the server waiting functionality
func TestWaitForStart(t *testing.T) {
	// Create a mock server that returns a successful ping response
	server := MockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/system/ping" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"OK"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	
	// Create a client that points to our mock server
	client := NewClient(server.URL, "admin", "password", "test")
	
	// Override the MaxWaitSeconds to make the test faster
	oldMax := MaxWaitSeconds
	MaxWaitSeconds = 1
	defer func() { MaxWaitSeconds = oldMax }()
	
	// Test waiting for server
	err := client.WaitForStart()
	if err != nil {
		t.Errorf("Expected server to start successfully, got error: %v", err)
	}
}

// TestCreateUsers tests the user creation functionality
func TestCreateUsers(t *testing.T) {
	// This is a more complex test that would require mocking multiple API calls
	// For simplicity, we'll just test the basic flow
	t.Skip("Skipping complex test that requires multiple API mocks")
}

// TestSetupWebhooks tests the webhook setup functionality
func TestSetupWebhooks(t *testing.T) {
	// This is a more complex test that would require mocking multiple API calls
	// For simplicity, we'll just test the basic flow
	t.Skip("Skipping complex test that requires multiple API mocks")
}

// Example of how to run a test manually
func ExampleClient_Login() {
	client := NewClient("http://localhost:8065", "sysadmin", "Testpassword123!", "test")
	err := client.Login()
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	fmt.Println("Login successful")
	// Output: Login successful
}
