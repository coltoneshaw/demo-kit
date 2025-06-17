package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// PluginManager handles all plugin-related operations
type PluginManager struct {
	client *Client
}

// NewPluginManager creates a new PluginManager instance
func NewPluginManager(client *Client) *PluginManager {
	return &PluginManager{client: client}
}

// Local plugins to build from source
func (pm *PluginManager) getLocalPlugins() []struct {
	ID   string
	Name string
	Path string
} {
	return []struct {
		ID   string
		Name string
		Path string
	}{
		{"com.coltoneshaw.weather", "Weather Plugin", "./apps/weather-plugin"},
		{"com.coltoneshaw.flightaware", "FlightAware Plugin", "./apps/flightaware-plugin"},
		{"com.coltoneshaw.missionops", "Mission Operations Plugin", "./apps/missionops-plugin"},
	}
}

// SetupLatestPlugins is the main entry point - downloads, builds, and installs all plugins
func (pm *PluginManager) SetupLatestPlugins(config *Config, forcePlugins, forceGitHubPlugins bool) error {
	return pm.SetupLatestPluginsWithUpdate(config, forcePlugins, forceGitHubPlugins, false)
}

// SetupLatestPluginsWithUpdate is the main entry point with update checking
func (pm *PluginManager) SetupLatestPluginsWithUpdate(config *Config, forcePlugins, forceGitHubPlugins, checkUpdates bool) error {
	if forceGitHubPlugins {
		fmt.Println("Reinstalling all plugins...")
	} else if forcePlugins {
		fmt.Println("Reinstalling local plugins...")
	} else {
		fmt.Println("Setting up plugins...")
	}

	// 1. Download GitHub plugins
	pm.downloadGitHubPluginsWithUpdate(config, forceGitHubPlugins, checkUpdates)

	// 2. Build local plugins
	pm.buildLocalPlugins(forcePlugins)

	// 3. Install everything from plugins directory (only force if forceGitHubPlugins)
	pm.installFromDirectory(forceGitHubPlugins)

	return nil
}

// downloadGitHubPlugins downloads configured GitHub plugins if not already installed
func (pm *PluginManager) downloadGitHubPlugins(config *Config, force bool) {
	pm.downloadGitHubPluginsWithUpdate(config, force, false)
}

// downloadGitHubPluginsWithUpdate downloads configured GitHub plugins with version checking
func (pm *PluginManager) downloadGitHubPluginsWithUpdate(config *Config, force, checkUpdates bool) {
	if config == nil || len(config.Plugins) == 0 {
		return
	}

	if force {
		fmt.Println("Re-downloading all GitHub plugins...")
	} else {
		fmt.Println("Checking GitHub plugins...")
	}

	for _, plugin := range config.Plugins {
		if !force {
			if checkUpdates {
				// Check if update is available
				needsUpdate, err := pm.needsUpdate(plugin.Name, plugin.Repo)
				if err != nil {
					fmt.Printf("⚠️ Failed to check updates for %s: %v\n", plugin.Name, err)
					// Continue with normal install check
					if pm.isInstalled(plugin.Name) {
						fmt.Printf("⏭️ Skipping %s: already installed\n", plugin.Name)
						continue
					}
				} else if !needsUpdate {
					fmt.Printf("⏭️ Skipping %s: already up to date\n", plugin.Name)
					continue
				} else {
					fmt.Printf("📦 Updating %s to newer version...\n", plugin.Name)
				}
			} else if pm.isInstalled(plugin.Name) {
				fmt.Printf("⏭️ Skipping %s: already installed\n", plugin.Name)
				continue
			}
		}

		if force {
			fmt.Printf("🔄 Re-downloading %s...\n", plugin.Name)
		} else {
			fmt.Printf("📦 Downloading %s...\n", plugin.Name)
		}

		if err := pm.downloadPlugin(plugin); err != nil {
			fmt.Printf("❌ Failed to download %s: %v\n", plugin.Name, err)
		}
	}
}

// buildLocalPlugins builds local plugins if not already installed
func (pm *PluginManager) buildLocalPlugins(force bool) {
	if !force {
		fmt.Println("Checking local plugins...")
	}

	for _, plugin := range pm.getLocalPlugins() {
		if !force && pm.isInstalled(plugin.Name) {
			fmt.Printf("⏭️ Skipping %s: already installed\n", plugin.Name)
			continue
		}

		if force {
			fmt.Printf("🔄 Rebuilding %s...\n", plugin.Name)
			// Clean before rebuilding when forced
			if err := pm.cleanPlugin(plugin.Path); err != nil {
				fmt.Printf("⚠️ Warning: Failed to clean %s: %v\n", plugin.Name, err)
			}
		} else {
			fmt.Printf("📦 Building %s...\n", plugin.Name)
		}

		if err := pm.buildPlugin(plugin.Path); err != nil {
			fmt.Printf("❌ Failed to build %s: %v\n", plugin.Name, err)
		}
	}
}

// installFromDirectory installs all .tar.gz files from plugins directory
func (pm *PluginManager) installFromDirectory(force bool) {
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return // No plugins directory, skip
	}

	if force {
		fmt.Println("Reinstalling all plugins from directory...")
	} else {
		fmt.Println("Installing plugins from directory...")
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".tar.gz") {
			continue
		}

		// Simple check: if filename looks like it's already installed, skip (unless forced)
		if !force && pm.isFileInstalled(file.Name()) {
			fmt.Printf("⏭️ Skipping %s: already installed\n", file.Name())
			continue
		}

		if force {
			fmt.Printf("🔄 Reinstalling %s...\n", file.Name())
		} else {
			fmt.Printf("📦 Installing %s...\n", file.Name())
		}

		pluginPath := filepath.Join(pluginsDir, file.Name())
		if err := pm.uploadPlugin(pluginPath); err != nil {
			fmt.Printf("❌ Failed to install %s: %v\n", file.Name(), err)
		}
	}
}

// Helper methods - simplified versions

// isInstalled checks if a plugin is installed (by name or common IDs)
func (pm *PluginManager) isInstalled(pluginName string) bool {
	plugins, _, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		return false // Assume not installed if we can't check
	}

	// Check by common plugin IDs for this name
	possibleIDs := pm.getPossibleIDs(pluginName)
	allPlugins := append(plugins.Active, plugins.Inactive...)

	for _, plugin := range allPlugins {
		for _, id := range possibleIDs {
			if plugin.Id == id {
				return true
			}
		}
	}
	return false
}

// isFileInstalled simple check if a tar.gz file represents an already installed plugin
func (pm *PluginManager) isFileInstalled(filename string) bool {
	// Extract plugin name from filename for basic checking
	name := strings.TrimSuffix(filename, ".tar.gz")

	// Check against config plugins
	if pm.client.Config != nil {
		for _, plugin := range pm.client.Config.Plugins {
			if plugin.PluginID != "" && strings.HasPrefix(name, plugin.PluginID) {
				return pm.isInstalled(plugin.Name)
			}
		}
	} else {
		// Hardcoded detection for common plugins since we don't have config
		if strings.Contains(name, "mattermost-ai") || strings.Contains(name, "plugin-ai") {
			return pm.isInstalled("mattermost-plugin-ai")
		}
		if strings.Contains(name, "playbooks") {
			return pm.isInstalled("mattermost-plugin-playbooks")
		}
	}

	return false // If we can't determine, install it
}

// getPossibleIDs returns likely plugin IDs for a plugin name from config and local plugins
func (pm *PluginManager) getPossibleIDs(pluginName string) []string {
	var ids []string

	// Check GitHub plugins from config
	if pm.client.Config != nil {
		for _, plugin := range pm.client.Config.Plugins {
			if plugin.Name == pluginName && plugin.PluginID != "" {
				// Add plugin-id and common variations
				ids = append(ids, plugin.PluginID)
				if strings.Contains(plugin.PluginID, "playbooks") {
					ids = append(ids, "playbooks", "com.mattermost.plugin-playbooks")
				}
				if strings.Contains(plugin.PluginID, "ai") || strings.Contains(plugin.PluginID, "agents") {
					ids = append(ids, "mattermost-ai", "com.mattermost.plugin-ai", "com.mattermost.plugin-agents")
				}
			}
		}
	} else {
		// Add hardcoded common GitHub plugin IDs since we don't have config
		switch pluginName {
		case "mattermost-plugin-ai":
			ids = append(ids, "mattermost-ai", "com.mattermost.plugin-ai")
		case "mattermost-plugin-playbooks":
			ids = append(ids, "playbooks", "com.mattermost.plugin-playbooks")
		}
	}

	// Check local plugins
	for _, plugin := range pm.getLocalPlugins() {
		if plugin.Name == pluginName {
			ids = append(ids, plugin.ID)
		}
	}

	return ids
}

// needsUpdate checks if a plugin needs updating by comparing server version with GitHub latest
func (pm *PluginManager) needsUpdate(pluginName, repo string) (bool, error) {
	// First check if plugin is installed
	if !pm.isInstalled(pluginName) {
		return true, nil // Not installed, so yes it needs "updating" (installing)
	}

	// Get installed version
	installedVersion, err := pm.getInstalledVersion(pluginName)
	if err != nil {
		return false, fmt.Errorf("failed to get installed version: %w", err)
	}

	// Get latest GitHub version
	latestVersion, err := pm.getLatestGitHubVersion(repo)
	if err != nil {
		return false, fmt.Errorf("failed to get latest GitHub version: %w", err)
	}

	// Compare versions
	return pm.isNewerVersion(latestVersion, installedVersion), nil
}

// getInstalledVersion gets the version of an installed plugin
func (pm *PluginManager) getInstalledVersion(pluginName string) (string, error) {
	plugins, _, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		return "", err
	}

	// Check by common plugin IDs for this name
	possibleIDs := pm.getPossibleIDs(pluginName)
	allPlugins := append(plugins.Active, plugins.Inactive...)

	for _, plugin := range allPlugins {
		for _, id := range possibleIDs {
			if plugin.Id == id {
				return plugin.Version, nil
			}
		}
	}
	return "", fmt.Errorf("plugin %s not found", pluginName)
}

// getLatestGitHubVersion gets the latest release version from GitHub
func (pm *PluginManager) getLatestGitHubVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

// isNewerVersion compares two version strings and returns true if version1 is newer than version2
// Supports semantic versioning (v1.2.3) and simple numeric versions
func (pm *PluginManager) isNewerVersion(version1, version2 string) bool {
	// Normalize versions by removing 'v' prefix
	v1 := strings.TrimPrefix(version1, "v")
	v2 := strings.TrimPrefix(version2, "v")

	// Split versions into parts
	v1Parts := strings.Split(v1, ".")
	v2Parts := strings.Split(v2, ".")

	// Compare each part
	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	for i := 0; i < maxLen; i++ {
		var part1, part2 int
		var err error

		if i < len(v1Parts) {
			part1, err = strconv.Atoi(v1Parts[i])
			if err != nil {
				return false // Invalid version format
			}
		}

		if i < len(v2Parts) {
			part2, err = strconv.Atoi(v2Parts[i])
			if err != nil {
				return false // Invalid version format
			}
		}

		if part1 > part2 {
			return true
		} else if part1 < part2 {
			return false
		}
		// If equal, continue to next part
	}

	// All parts are equal
	return false
}

// downloadPlugin downloads a plugin from GitHub
func (pm *PluginManager) downloadPlugin(plugin PluginConfig) error {
	// Get latest release
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", plugin.Repo)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}

	// Find .tar.gz asset
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".tar.gz") && !strings.Contains(asset.Name, "linux") &&
			!strings.Contains(asset.Name, "darwin") && !strings.Contains(asset.Name, "windows") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no suitable .tar.gz found")
	}

	// Download to plugins directory
	return pm.downloadFile(downloadURL, plugin.PluginID+"-"+release.TagName+".tar.gz")
}

// downloadFile downloads a file to the plugins directory
func (pm *PluginManager) downloadFile(url, filename string) error {
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	_ = os.MkdirAll(pluginsDir, 0755)
	filePath := filepath.Join(pluginsDir, filename)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, resp.Body)
	return err
}

// cleanPlugin cleans a plugin build directory
func (pm *PluginManager) cleanPlugin(pluginPath string) error {
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	cmd := exec.Command("make", "clean")
	cmd.Dir = pluginPath
	return cmd.Run()
}

// buildPlugin builds a plugin from source
func (pm *PluginManager) buildPlugin(pluginPath string) error {
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	return cmd.Run()
}

// uploadPlugin uploads and enables a plugin
func (pm *PluginManager) uploadPlugin(bundlePath string) error {
	file, err := os.Open(bundlePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	manifest, _, err := pm.client.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		return err
	}

	_, err = pm.client.API.EnablePlugin(context.Background(), manifest.Id)
	return err
}
