package main

import (
	"fmt"
	"os"
	"os/exec"
)

const dirPath = "./volumes/keycloak"

func restore() {
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found skipping keycloak setup\n", dirPath)
		fmt.Println()
		fmt.Println("===========================================================")
	} else {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("Warning: '%s' NOT found. Setting up from base\n", dirPath)
		fmt.Println()
		fmt.Println("===========================================================")

		// Create directory
		if err := os.MkdirAll("./volumes/keycloak", 0755); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			os.Exit(1)
		}

		// Extract backup
		cmd := exec.Command("tar", "-zxf", "./files/keycloak/keycloakBackup.tar", "-C", "./volumes/keycloak")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error extracting backup: %v\n", err)
			os.Exit(1)
		}
	}
}

func backup() {
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found backing up keycloak\n", dirPath)
		fmt.Println()
		fmt.Println("===========================================================")

		// Create backup
		cmd := exec.Command("tar", "-zcf", "keycloakBackup.tar", "-C", "./volumes/keycloak", ".")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error creating backup: %v\n", err)
			os.Exit(1)
		}

		// Move backup file
		if err := os.Rename("keycloakBackup.tar", "./files/keycloak/keycloakBackup.tar"); err != nil {
			fmt.Printf("Error moving backup file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("Warning: '%s' NOT found. Skipping backup\n", dirPath)
		fmt.Println()
		fmt.Println("===========================================================")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a command: restore or backup")
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "restore":
		restore()
	case "backup":
		backup()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}
