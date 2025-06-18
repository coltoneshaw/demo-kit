package cmd

import (
	"github.com/coltoneshaw/demokit/mattermost"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	reinstallPlugins  string
	checkUpdates      bool
	setupLdap         bool
	ldapURL           string
	ldapBindDN        string
	ldapBindPassword  string
	ldapBaseDN        string
)

// setupCmd represents the setup command
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup users, teams, channels, and slash commands",
	Long: `Setup users, teams, channels, and slash commands on a Mattermost server.

This command will:
- Create teams and channels from bulk_import.jsonl
- Create users and add them to teams/channels
- Execute any configured channel commands

The setup process uses the connection flags to connect to the target Mattermost server.
It uses a two-phase bulk import system (infrastructure first, then users) for optimal reliability.

Plugin Options:
  --reinstall-plugins local   Rebuild and redeploy custom local plugins only
  --reinstall-plugins all     Rebuild all plugins and redeploy everything
  --check-updates             Check for and install newer plugin versions from GitHub

LDAP Options:
  --ldap                      Setup LDAP directory and migrate existing users to LDAP auth
  --ldap-url                  LDAP server URL (default: ldap://localhost:10389)
  --ldap-bind-dn              LDAP admin bind DN (default: cn=admin,dc=planetexpress,dc=com)
  --ldap-bind-password        LDAP admin password (default: GoodNewsEveryone)
  --ldap-base-dn              LDAP base DN (default: dc=planetexpress,dc=com)`,
	Run: func(cmd *cobra.Command, args []string) {
		client := mattermost.NewClient(serverURL, adminUser, adminPass, teamName, configPath)

		// Validate reinstall-plugins option
		if reinstallPlugins != "" && reinstallPlugins != "local" && reinstallPlugins != "all" {
			mattermost.Log.WithFields(logrus.Fields{
				"provided": reinstallPlugins,
				"valid_options": []string{"local", "all"},
			}).Fatal("Invalid reinstall-plugins option")
		}

		forcePlugins := reinstallPlugins == "local" || reinstallPlugins == "all"
		forceGitHubPlugins := reinstallPlugins == "all"
		forceAll := false // Data import forcing would need a separate flag

		if err := client.SetupWithForceAndUpdates(forcePlugins, forceGitHubPlugins, forceAll, checkUpdates); err != nil {
			mattermost.Log.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Fatal("Setup failed")
		}

		// Setup LDAP if requested
		if setupLdap {
			// Load LDAP configuration from config file and CLI flags
			ldapConfig, err := buildLDAPConfig(client.Config)
			if err != nil {
				mattermost.Log.WithFields(logrus.Fields{
					"error": err.Error(),
				}).Fatal("Failed to build LDAP configuration")
			}

			if err := client.SetupLDAPWithConfig(ldapConfig); err != nil {
				mattermost.Log.WithFields(logrus.Fields{
					"error": err.Error(),
				}).Fatal("LDAP setup failed")
			}
		}
	},
}

// buildLDAPConfig creates an LDAPConfig from config file and CLI flags
// CLI flags take precedence over config file values
func buildLDAPConfig(config *mattermost.Config) (*mattermost.LDAPConfig, error) {
	ldapConfig := &mattermost.LDAPConfig{}

	// Start with config file values if available
	if config != nil && config.LDAP.URL != "" {
		ldapConfig.URL = config.LDAP.URL
		ldapConfig.BindDN = config.LDAP.BindDN
		ldapConfig.BindPassword = config.LDAP.BindPassword
		ldapConfig.BaseDN = config.LDAP.BaseDN
		ldapConfig.SchemaBindDN = config.LDAP.SchemaBindDN
		ldapConfig.SchemaPassword = config.LDAP.SchemaPassword
	}

	// Override with CLI flags if provided
	if ldapURL != "" {
		ldapConfig.URL = ldapURL
	}
	if ldapBindDN != "" {
		ldapConfig.BindDN = ldapBindDN
	}
	if ldapBindPassword != "" {
		ldapConfig.BindPassword = ldapBindPassword
	}
	if ldapBaseDN != "" {
		ldapConfig.BaseDN = ldapBaseDN
	}

	// Set defaults for required fields if still empty
	if ldapConfig.URL == "" {
		ldapConfig.URL = "ldap://localhost:10389"
	}
	if ldapConfig.BindDN == "" {
		ldapConfig.BindDN = "cn=admin,dc=planetexpress,dc=com"
	}
	if ldapConfig.BindPassword == "" {
		ldapConfig.BindPassword = "GoodNewsEveryone"
	}
	if ldapConfig.BaseDN == "" {
		ldapConfig.BaseDN = "dc=planetexpress,dc=com"
	}
	if ldapConfig.SchemaBindDN == "" {
		ldapConfig.SchemaBindDN = "cn=admin,cn=config"
	}
	if ldapConfig.SchemaPassword == "" {
		ldapConfig.SchemaPassword = "GoodNewsEveryone"
	}

	return ldapConfig, nil
}

func init() {
	RootCmd.AddCommand(setupCmd)
	
	// Add the reinstall-plugins flag
	setupCmd.Flags().StringVar(&reinstallPlugins, "reinstall-plugins", "", "Plugin reinstall options: 'local' (rebuild custom plugins only), 'all' (rebuild all plugins)")
	
	// Add the check-updates flag
	setupCmd.Flags().BoolVar(&checkUpdates, "check-updates", false, "Check for and install newer plugin versions from GitHub")
	
	// Add the ldap flags
	setupCmd.Flags().BoolVar(&setupLdap, "ldap", false, "Setup LDAP directory and migrate existing users to LDAP auth")
	setupCmd.Flags().StringVar(&ldapURL, "ldap-url", "", "LDAP server URL (default: ldap://localhost:10389)")
	setupCmd.Flags().StringVar(&ldapBindDN, "ldap-bind-dn", "", "LDAP admin bind DN (default: cn=admin,dc=planetexpress,dc=com)")
	setupCmd.Flags().StringVar(&ldapBindPassword, "ldap-bind-password", "", "LDAP admin password (default: GoodNewsEveryone)")
	setupCmd.Flags().StringVar(&ldapBaseDN, "ldap-base-dn", "", "LDAP base DN (default: dc=planetexpress,dc=com)")
}
