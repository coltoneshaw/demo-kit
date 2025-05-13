package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yourusername/yourproject/mattermost"
)

func main() {
	// Define command line flags
	setupCmd := flag.Bool("setup", false, "Run the setup process")
	echoLoginsCmd := flag.Bool("echo-logins", false, "Display login information")
	serverURL := flag.String("server", "http://localhost:8065", "Mattermost server URL")
	adminUser := flag.String("admin", "sysadmin", "Admin username")
	adminPass := flag.String("password", "Testpassword123!", "Admin password")
	teamName := flag.String("team", "test", "Team name")
	
	flag.Parse()
	
	// Create a new Mattermost client
	client := mattermost.NewClient(*serverURL, *adminUser, *adminPass, *teamName)
	
	// Execute the requested command
	if *setupCmd {
		if err := client.Setup(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
	} else if *echoLoginsCmd {
		client.EchoLogins()
	} else {
		fmt.Println("No command specified. Use -setup or -echo-logins")
		os.Exit(1)
	}
}
