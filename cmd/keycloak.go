package cmd

import (
	"fmt"
	"os"
	"os/exec"
)

const KeycloakDirPath = "./volumes/keycloak"

// RestoreCmdF sets up Keycloak from backup if needed
func RestoreCmdF() error {
	if _, err := os.Stat(KeycloakDirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found skipping keycloak setup\n", KeycloakDirPath)
		fmt.Println()
		fmt.Println("===========================================================")
		return nil
	}
	
	fmt.Println("===========================================================")
	fmt.Println()
	fmt.Printf("Warning: '%s' NOT found. Setting up from base\n", KeycloakDirPath)
	fmt.Println()
	fmt.Println("===========================================================")

	// Create directory
	if err := os.MkdirAll("./volumes/keycloak", 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	// Extract backup
	cmd := exec.Command("tar", "-zxf", "./files/keycloak/keycloakBackup.tar", "-C", "./volumes/keycloak")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error extracting backup: %v", err)
	}
	
	return nil
}

// BackupCmdF creates a backup of Keycloak data
func BackupCmdF() error {
	if _, err := os.Stat(KeycloakDirPath); !os.IsNotExist(err) {
		fmt.Println("===========================================================")
		fmt.Println()
		fmt.Printf("'%s' found backing up keycloak\n", KeycloakDirPath)
		fmt.Println()
		fmt.Println("===========================================================")

		// Create backup
		cmd := exec.Command("tar", "-zcf", "keycloakBackup.tar", "-C", "./volumes/keycloak", ".")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error creating backup: %v", err)
		}

		// Move backup file
		if err := os.Rename("keycloakBackup.tar", "./files/keycloak/keycloakBackup.tar"); err != nil {
			return fmt.Errorf("error moving backup file: %v", err)
		}
		
		return nil
	}
	
	fmt.Println("===========================================================")
	fmt.Println()
	fmt.Printf("Warning: '%s' NOT found. Skipping backup\n", KeycloakDirPath)
	fmt.Println()
	fmt.Println("===========================================================")
	
	return nil
}
