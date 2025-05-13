package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const MattermostDirPath = "./volumes/mattermost"
const MaxWaitSeconds = 120

// MattermostCmd represents the mattermost command
var MattermostCmd = &cobra.Command{
	Use:   "mattermost",
	Short: "Mattermost management commands",
	Long:  `Commands for managing Mattermost setup and configuration.`,
}

// SetupCmd sets up Mattermost with test data
var SetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up Mattermost with test data",
	Long:  `Configure Mattermost with test users, teams, and webhooks.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return SetupCmdF()
	},
}

// EchoLoginsCmd displays Mattermost login information
var EchoLoginsCmd = &cobra.Command{
	Use:   "echologins",
	Short: "Display Mattermost login information",
	Long:  `Display login credentials for Mattermost users.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return EchoLoginsCmdF()
	},
}

func init() {
	RootCmd.AddCommand(MattermostCmd)
	MattermostCmd.AddCommand(SetupCmd)
	MattermostCmd.AddCommand(EchoLoginsCmd)
}

// WaitForStart waits for the Mattermost server to start
func WaitForStart() bool {
	fmt.Printf("waiting %d seconds for the server to start\n", MaxWaitSeconds)

	total := 0
	for total <= MaxWaitSeconds {
		cmd := exec.Command("docker", "exec", "-i", "mattermost", "mmctl", "system", "status", "--local")
		cmd.Stderr = nil
		if err := cmd.Run(); err == nil {
			fmt.Println("server started")
			return true
		}

		total++
		fmt.Print(".")
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("\nserver didn't start in %d seconds\n", MaxWaitSeconds)

	stopCmd := exec.Command("make", "stop")
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	stopCmd.Run()

	return false
}

// SetupCmdF configures the Mattermost server with test data
func SetupCmdF() error {
	if !WaitForStart() {
		cmd := exec.Command("make", "stop")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		return fmt.Errorf("server failed to start within timeout period")
	}

	fmt.Println("===========================================================")
	fmt.Println()
	fmt.Println("setting up test Data for Mattermost")
	fmt.Println()
	fmt.Println("===========================================================")

	// Check if sysadmin user exists before creating
	checkSysadmin := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "user", "list", "--local")
	sysadminOutput, _ := checkSysadmin.CombinedOutput()
	if !strings.Contains(string(sysadminOutput), "sysadmin") {
		fmt.Println("Creating sysadmin user...")
		createSysadmin := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "user", "create",
			"--password", "Testpassword123!", "--username", "sysadmin", "--email", "sysadmin@example.com",
			"--system-admin", "--local")
		createSysadmin.Stdout = os.Stdout
		createSysadmin.Stderr = os.Stderr
		createSysadmin.Run()
	} else {
		fmt.Println("User 'sysadmin' already exists")
	}

	// Check if user-1 exists before creating
	checkUser1 := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "user", "list", "--local")
	user1Output, _ := checkUser1.CombinedOutput()
	if !strings.Contains(string(user1Output), "user-1") {
		fmt.Println("Creating user-1 user...")
		createUser1 := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "user", "create",
			"--password", "Testpassword123!", "--username", "user-1", "--email", "user-1@example.com", "--local")
		createUser1.Stdout = os.Stdout
		createUser1.Stderr = os.Stderr
		createUser1.Run()
	} else {
		fmt.Println("User 'user-1' already exists")
	}

	// Check if team exists before creating it
	checkTeam := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "team", "list", "--local")
	teamOutput, _ := checkTeam.CombinedOutput()
	if !strings.Contains(string(teamOutput), "test") {
		fmt.Println("Creating test team...")
		createTeam := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "team", "create",
			"--name", "test", "--display-name", "Test Team", "--local")
		createTeam.Stdout = os.Stdout
		createTeam.Stderr = os.Stderr
		createTeam.Run()
	} else {
		fmt.Println("Team 'test' already exists")
	}

	// Add users to the team
	addUsers := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "team", "users", "add",
		"test", "sysadmin", "user-1", "--local")
	addUsers.Stdout = os.Stdout
	addUsers.Stderr = os.Stderr
	addUsers.Run()

	// Get the channel ID for off-topic in the test team
	fmt.Println("Getting channel ID for off-topic in test team...")
	getChannelCmd := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "channel", "list", "test", "--local")
	channelOutput, _ := getChannelCmd.CombinedOutput()

	// Extract channel ID using regex
	re := regexp.MustCompile(`off-topic\s+(\w+)`)
	matches := re.FindStringSubmatch(string(channelOutput))

	if len(matches) > 1 {
		channelID := matches[1]
		fmt.Printf("Found off-topic channel ID: %s\n", channelID)

		// Check if webhook already exists
		checkWebhook := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "webhook", "list-incoming", "--local")
		webhookOutput, _ := checkWebhook.CombinedOutput()

		if !strings.Contains(string(webhookOutput), "weather") {
			fmt.Println("Creating incoming webhook for weather app...")
			createWebhook := exec.Command("docker", "exec", "-it", "mattermost", "mmctl", "webhook", "create-incoming",
				"--channel", channelID, "--user", "sysadmin", "--display-name", "weather",
				"--description", "Weather responses", "--icon", "http://weather-app:8085/bot.png", "--local")
			webhookOutputBytes, _ := createWebhook.CombinedOutput()
			webhookOutput := string(webhookOutputBytes)

			// Extract webhook ID using regex
			reWebhook := regexp.MustCompile(`Id: (\w+)`)
			webhookMatches := reWebhook.FindStringSubmatch(webhookOutput)

			if len(webhookMatches) > 1 {
				webhookID := webhookMatches[1]
				fmt.Printf("Created webhook with ID: %s\n", webhookID)

				// Update env_vars.env file with the webhook URL
				webhookURL := fmt.Sprintf("http://mattermost:8065/hooks/%s", webhookID)
				fmt.Printf("Setting webhook URL: %s\n", webhookURL)

				// Read the env_vars.env file
				envFile := "./files/env_vars.env"
				content, err := ioutil.ReadFile(envFile)
				if err != nil {
					return fmt.Errorf("error reading env file: %v", err)
				}

				// Replace the webhook URL line
				newContent := regexp.MustCompile(`MATTERMOST_WEBHOOK_URL=.*`).
					ReplaceAllString(string(content), fmt.Sprintf("MATTERMOST_WEBHOOK_URL=%s", webhookURL))

				// Write the updated content back to the file
				err = ioutil.WriteFile(envFile, []byte(newContent), 0644)
				if err != nil {
					return fmt.Errorf("error writing env file: %v", err)
				}

				fmt.Println("Updated env_vars.env with webhook URL")

				// Restart the weather-app container
				fmt.Println("Restarting weather-app container...")
				restartCmd := exec.Command("docker", "restart", "weather-app")
				restartCmd.Stdout = os.Stdout
				restartCmd.Stderr = os.Stderr
				restartCmd.Run()
				fmt.Println("Weather app restarted successfully")
			} else {
				fmt.Println("Failed to create webhook")
			}
		} else {
			fmt.Println("Webhook 'weather' already exists")
		}
	} else {
		fmt.Println("Could not find off-topic channel in test team")
	}

	fmt.Println()
	fmt.Println("Alright, everything seems to be setup and running. Enjoy.")

	return nil
}

// EchoLoginsCmdF prints login information for Mattermost users
func EchoLoginsCmdF() error {
	fmt.Println()
	fmt.Println("========================================================================")
	fmt.Println()
	fmt.Println("Mattermost logins:")
	fmt.Println()
	fmt.Println("- System admin")
	fmt.Println("     - username: sysadmin")
	fmt.Println("     - password: Testpassword123!")
	fmt.Println("- Regular account:")
	fmt.Println("     - username: user-1")
	fmt.Println("     - password: Testpassword123!")
	fmt.Println("- LDAP or SAML account:")
	fmt.Println("     - username: professor")
	fmt.Println("     - password: professor")
	fmt.Println()
	fmt.Println("For more logins check out https://github.com/coltoneshaw/mattermost#accounts")
	fmt.Println()
	fmt.Println("========================================================================")

	return nil
}
