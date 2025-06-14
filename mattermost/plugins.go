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
		{"com.coltoneshaw.weather", "Weather Plugin", "../apps/weather-plugin"},
		{"com.coltoneshaw.flightaware", "FlightAware Plugin", "../apps/flightaware-plugin"},
		{"com.coltoneshaw.missionops", "Mission Operations Plugin", "../apps/missionops-plugin"},
	}
}

// SetupLatestPlugins is the main entry point - downloads, builds, and installs all plugins
func (pm *PluginManager) SetupLatestPlugins(config *Config) error {
	fmt.Println("Setting up all plugins...")

	// 1. Download GitHub plugins
	pm.downloadGitHubPlugins(config)

	// 2. Build local plugins  
	pm.buildLocalPlugins()

	// 3. Install everything from plugins directory
	pm.installFromDirectory()

	return nil
}

// downloadGitHubPlugins downloads configured GitHub plugins if not already installed
func (pm *PluginManager) downloadGitHubPlugins(config *Config) {
	if config == nil || len(config.Plugins) == 0 {
		return
	}

	fmt.Println("Checking GitHub plugins...")
	for _, plugin := range config.Plugins {
		if pm.isInstalled(plugin.Name) {
			fmt.Printf("⏭️ Skipping %s: already installed\n", plugin.Name)
			continue
		}

		fmt.Printf("📦 Downloading %s...\n", plugin.Name)
		if err := pm.downloadPlugin(plugin); err != nil {
			fmt.Printf("❌ Failed to download %s: %v\n", plugin.Name, err)
		}
	}
}

// buildLocalPlugins builds local plugins if not already installed
func (pm *PluginManager) buildLocalPlugins() {
	fmt.Println("Checking local plugins...")
	for _, plugin := range pm.getLocalPlugins() {
		if pm.isInstalled(plugin.Name) {
			fmt.Printf("⏭️ Skipping %s: already installed\n", plugin.Name)
			continue
		}

		fmt.Printf("📦 Building %s...\n", plugin.Name)
		if err := pm.buildPlugin(plugin.Path); err != nil {
			fmt.Printf("❌ Failed to build %s: %v\n", plugin.Name, err)
		}
	}
}

// installFromDirectory installs all .tar.gz files from plugins directory
func (pm *PluginManager) installFromDirectory() {
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return // No plugins directory, skip
	}

	fmt.Println("Installing plugins from directory...")
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".tar.gz") {
			continue
		}

		// Simple check: if filename looks like it's already installed, skip
		if pm.isFileInstalled(file.Name()) {
			fmt.Printf("⏭️ Skipping %s: already installed\n", file.Name())
			continue
		}

		fmt.Printf("📦 Installing %s...\n", file.Name())
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
	}
	
	// Check local plugins
	for _, plugin := range pm.getLocalPlugins() {
		if plugin.Name == pluginName {
			ids = append(ids, plugin.ID)
		}
	}
	
	return ids
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