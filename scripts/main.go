package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a command and subcommand")
		fmt.Println("Usage: ./scripts/main [general|keycloak|mattermost] [subcommand]")
		os.Exit(1)
	}

	command := os.Args[1]

	// Shift arguments to remove the first one
	var subArgs []string
	if len(os.Args) > 2 {
		subArgs = os.Args[2:]
	}

	switch strings.ToLower(command) {
	case "general":
		handleGeneralCommand(subArgs)
	case "keycloak":
		handleKeycloakCommand(subArgs)
	case "mattermost":
		handleMattermostCommand(subArgs)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: general, keycloak, mattermost")
		os.Exit(1)
	}
}

func handleGeneralCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Please provide a subcommand for general")
		fmt.Println("Available subcommands: logins")
		os.Exit(1)
	}

	subcommand := args[0]
	switch strings.ToLower(subcommand) {
	case "logins":
		general.Logins()
	default:
		fmt.Printf("Unknown subcommand for general: %s\n", subcommand)
		fmt.Println("Available subcommands: logins")
		os.Exit(1)
	}
}

func handleKeycloakCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Please provide a subcommand for keycloak")
		fmt.Println("Available subcommands: restore, backup")
		os.Exit(1)
	}

	subcommand := args[0]
	switch strings.ToLower(subcommand) {
	case "restore":
		keycloak.Restore()
	case "backup":
		keycloak.Backup()
	default:
		fmt.Printf("Unknown subcommand for keycloak: %s\n", subcommand)
		fmt.Println("Available subcommands: restore, backup")
		os.Exit(1)
	}
}

func handleMattermostCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Please provide a subcommand for mattermost")
		fmt.Println("Available subcommands: setup, echologins")
		os.Exit(1)
	}

	subcommand := args[0]
	switch strings.ToLower(subcommand) {
	case "setup":
		mattermost.Setup()
	case "echologins":
		mattermost.EchoLogins()
	default:
		fmt.Printf("Unknown subcommand for mattermost: %s\n", subcommand)
		fmt.Println("Available subcommands: setup, echologins")
		os.Exit(1)
	}
}
