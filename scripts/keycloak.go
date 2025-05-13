package keycloak

import (
	"fmt"
	"os"
	"os/exec"
)

const DirPath = "./volumes/keycloak"

// Restore sets up Keycloak from backup if needed
func Restore() {
	if _, err := os.Stat(DirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found skipping keycloak setup\n", DirPath)
		fmt.Println()
		fmt.Println("===========================================================")
	} else {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("Warning: '%s' NOT found. Setting up from base\n", DirPath)
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

// Backup creates a backup of Keycloak data
func Backup() {
	if _, err := os.Stat(DirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found backing up keycloak\n", DirPath)
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
		fmt.Printf("Warning: '%s' NOT found. Skipping backup\n", DirPath)
		fmt.Println()
		fmt.Println("===========================================================")
	}
}
