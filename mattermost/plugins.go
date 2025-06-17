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

	"github.com/sirupsen/logrus"
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
		Log.WithFields(logrus.Fields{"force_plugins": forcePlugins, "force_github_plugins": forceGitHubPlugins, "check_updates": checkUpdates, "operation_type": "reinstall_all"}).Info("🚀 Reinstalling all plugins...")
	} else if forcePlugins {
		Log.WithFields(logrus.Fields{"force_plugins": forcePlugins, "force_github_plugins": forceGitHubPlugins, "check_updates": checkUpdates, "operation_type": "reinstall_local"}).Info("🚀 Reinstalling local plugins...")
	} else {
		Log.WithFields(logrus.Fields{"force_plugins": forcePlugins, "force_github_plugins": forceGitHubPlugins, "check_updates": checkUpdates, "operation_type": "setup"}).Info("🚀 Setting up plugins...")
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
// nolint:unused // This is a convenience wrapper that may be used in the future
func (pm *PluginManager) downloadGitHubPlugins(config *Config, force bool) {
	pm.downloadGitHubPluginsWithUpdate(config, force, false)
}

// downloadGitHubPluginsWithUpdate downloads configured GitHub plugins with version checking
func (pm *PluginManager) downloadGitHubPluginsWithUpdate(config *Config, force, checkUpdates bool) {
	
	if config == nil || len(config.Plugins) == 0 {
		Log.Debug("Debug: No plugins configured or config is nil")
		return
	}

	if force {
		Log.WithFields(logrus.Fields{"force_github_plugins": force, "check_updates": checkUpdates, "plugin_count": len(config.Plugins), "operation_type": "re-download"}).Info("📦 Re-downloading all GitHub plugins...")
	} else {
		Log.WithFields(logrus.Fields{"force_github_plugins": force, "check_updates": checkUpdates, "plugin_count": len(config.Plugins), "operation_type": "check"}).Info("📋 Checking GitHub plugins...")
	}

	for _, plugin := range config.Plugins {
		if !force {
			if checkUpdates {
				// Check if update is available
				needsUpdate, err := pm.needsUpdate(plugin.Name, plugin.Repo)
				if err != nil {
					Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "error": err.Error(), "operation_type": "check_update"}).Warn("⚠️ Failed to check updates for " + plugin.Name)
					// Continue with normal install check
					if pm.isInstalled(plugin.Name) {
						Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "operation_type": "skip"}).Info("⏭️ Skipping " + plugin.Name + ": already installed")
						continue
					}
				} else if !needsUpdate {
					Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "operation_type": "skip"}).Info("⏭️ Skipping " + plugin.Name + ": already up to date")
					continue
				} else {
					Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "operation_type": "update"}).Info("📦 Updating " + plugin.Name + " to newer version...")
				}
			} else if pm.isInstalled(plugin.Name) {
				Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "operation_type": "skip"}).Info("⏭️ Skipping " + plugin.Name + ": already installed")
				continue
			}
		}

		if force {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "operation_type": "re-download"}).Info("🔄 Re-downloading " + plugin.Name + "...")
		} else {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "operation_type": "download"}).Info("📦 Downloading " + plugin.Name + "...")
		}

		if err := pm.downloadPlugin(plugin); err != nil {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "error": err.Error(), "operation_type": "download"}).Error("❌ Failed to download " + plugin.Name)
		} else {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "operation_type": "download"}).Info("✅ Successfully downloaded " + plugin.Name)
		}
	}
}

// buildLocalPlugins builds local plugins if not already installed
func (pm *PluginManager) buildLocalPlugins(force bool) {
	
	if !force {
		Log.WithFields(logrus.Fields{"force_plugins": force, "local_plugin_count": len(pm.getLocalPlugins()), "operation_type": "check"}).Info("📋 Checking local plugins...")
	}

	for _, plugin := range pm.getLocalPlugins() {
		if !force && pm.isInstalled(plugin.Name) {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_id": plugin.ID, "plugin_path": plugin.Path, "operation_type": "skip"}).Info("⏭️ Skipping " + plugin.Name + ": already installed")
			continue
		}

		if force {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_id": plugin.ID, "plugin_path": plugin.Path, "operation_type": "rebuild"}).Info("🔄 Rebuilding " + plugin.Name + "...")
			// Clean before rebuilding when forced
			if err := pm.cleanPlugin(plugin.Path); err != nil {
				Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_path": plugin.Path, "error": err.Error(), "operation_type": "clean"}).Warn("⚠️ Warning: Failed to clean " + plugin.Name)
			}
		} else {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_id": plugin.ID, "plugin_path": plugin.Path, "operation_type": "build"}).Info("📦 Building " + plugin.Name + "...")
		}

		if err := pm.buildPlugin(plugin.Path); err != nil {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_id": plugin.ID, "plugin_path": plugin.Path, "error": err.Error(), "operation_type": "build"}).Error("❌ Failed to build " + plugin.Name)
		} else {
			Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "plugin_id": plugin.ID, "plugin_path": plugin.Path, "operation_type": "build"}).Info("✅ Successfully built " + plugin.Name)
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
		Log.WithFields(logrus.Fields{"plugins_dir": pluginsDir, "error": err.Error()}).Debug("Debug: No plugins directory found, skipping")
		return // No plugins directory, skip
	}

	if force {
		Log.WithFields(logrus.Fields{"plugins_dir": pluginsDir, "file_count": len(files), "force_plugins": force, "operation_type": "reinstall"}).Info("🚀 Reinstalling all plugins from directory...")
	} else {
		Log.WithFields(logrus.Fields{"plugins_dir": pluginsDir, "file_count": len(files), "force_plugins": force, "operation_type": "install"}).Info("📦 Installing plugins from directory...")
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".tar.gz") {
			continue
		}

		// Simple check: if filename looks like it's already installed, skip (unless forced)
		if !force && pm.isFileInstalled(file.Name()) {
			Log.WithFields(logrus.Fields{"file_name": file.Name(), "plugins_dir": pluginsDir, "operation_type": "skip"}).Info("⏭️ Skipping " + file.Name() + ": already installed")
			continue
		}

		if force {
			Log.WithFields(logrus.Fields{"file_name": file.Name(), "plugins_dir": pluginsDir, "operation_type": "reinstall"}).Info("🔄 Reinstalling " + file.Name() + "...")
		} else {
			Log.WithFields(logrus.Fields{"file_name": file.Name(), "plugins_dir": pluginsDir, "operation_type": "install"}).Info("📦 Installing " + file.Name() + "...")
		}

		pluginPath := filepath.Join(pluginsDir, file.Name())
		if err := pm.uploadPlugin(pluginPath); err != nil {
			Log.WithFields(logrus.Fields{"file_name": file.Name(), "plugin_path": pluginPath, "error": err.Error(), "operation_type": "install"}).Error("❌ Failed to install " + file.Name())
		} else {
			Log.WithFields(logrus.Fields{"file_name": file.Name(), "plugin_path": pluginPath, "operation_type": "install"}).Info("✅ Successfully installed " + file.Name())
		}
	}
}

// Helper methods - simplified versions

// isInstalled checks if a plugin is installed (by name or common IDs)
func (pm *PluginManager) isInstalled(pluginName string) bool {
	
	plugins, _, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "error": err.Error()}).Debug("Debug: Failed to get plugins list, assuming not installed")
		return false // Assume not installed if we can't check
	}

	// Check by common plugin IDs for this name
	possibleIDs := pm.getPossibleIDs(pluginName)
	allPlugins := append(plugins.Active, plugins.Inactive...)

	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "possible_ids": possibleIDs, "active_count": len(plugins.Active), "inactive_count": len(plugins.Inactive)}).Debug("Debug: Checking if plugin is installed")

	for _, plugin := range allPlugins {
		for _, id := range possibleIDs {
			if plugin.Id == id {
				Log.WithFields(logrus.Fields{"plugin_name": pluginName, "plugin_id": id, "plugin_version": plugin.Version}).Debug("Debug: Found installed plugin")
				return true
			}
		}
	}
	
	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "possible_ids": possibleIDs}).Debug("Debug: Plugin not found in installed list")
	return false
}

// isFileInstalled simple check if a tar.gz file represents an already installed plugin
func (pm *PluginManager) isFileInstalled(filename string) bool {
	
	// Extract plugin name from filename for basic checking
	name := strings.TrimSuffix(filename, ".tar.gz")
	
	Log.WithFields(logrus.Fields{"filename": filename, "extracted_name": name}).Debug("Debug: Checking if file represents installed plugin")

	// Check against config plugins
	if pm.client.Config != nil {
		Log.WithFields(logrus.Fields{"filename": filename, "config_plugin_count": len(pm.client.Config.Plugins)}).Debug("Debug: Checking against config plugins")
		for _, plugin := range pm.client.Config.Plugins {
			if plugin.PluginID != "" && strings.HasPrefix(name, plugin.PluginID) {
				Log.WithFields(logrus.Fields{"filename": filename, "plugin_name": plugin.Name, "plugin_id": plugin.PluginID}).Debug("Debug: Found matching config plugin, checking if installed")
				return pm.isInstalled(plugin.Name)
			}
		}
	} else {
		Log.WithFields(logrus.Fields{"filename": filename}).Debug("Debug: No config available, using hardcoded detection")
		// Hardcoded detection for common plugins since we don't have config
		if strings.Contains(name, "mattermost-ai") || strings.Contains(name, "plugin-ai") {
			Log.WithFields(logrus.Fields{"filename": filename, "detected_plugin": "mattermost-plugin-ai"}).Debug("Debug: Detected AI plugin")
			return pm.isInstalled("mattermost-plugin-ai")
		}
		if strings.Contains(name, "playbooks") {
			Log.WithFields(logrus.Fields{"filename": filename, "detected_plugin": "mattermost-plugin-playbooks"}).Debug("Debug: Detected Playbooks plugin")
			return pm.isInstalled("mattermost-plugin-playbooks")
		}
	}

	Log.WithFields(logrus.Fields{"filename": filename}).Debug("Debug: Could not determine plugin type, assuming not installed")
	return false // If we can't determine, install it
}

// getPossibleIDs returns likely plugin IDs for a plugin name from config and local plugins
func (pm *PluginManager) getPossibleIDs(pluginName string) []string {
	var ids []string

	Log.WithFields(logrus.Fields{"plugin_name": pluginName}).Debug("Debug: Getting possible IDs for plugin")

	// Check GitHub plugins from config
	if pm.client.Config != nil {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "config_plugin_count": len(pm.client.Config.Plugins)}).Debug("Debug: Checking config plugins for ID variations")
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
				Log.WithFields(logrus.Fields{"plugin_name": pluginName, "base_plugin_id": plugin.PluginID}).Debug("Debug: Found config plugin, added ID variations")
			}
		}
	} else {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName}).Debug("Debug: No config available, using hardcoded plugin IDs")
		// Add hardcoded common GitHub plugin IDs since we don't have config
		switch pluginName {
		case "mattermost-plugin-ai":
			ids = append(ids, "mattermost-ai", "com.mattermost.plugin-ai")
		case "mattermost-plugin-playbooks":
			ids = append(ids, "playbooks", "com.mattermost.plugin-playbooks")
		}
	}

	// Check local plugins
	localPlugins := pm.getLocalPlugins()
	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "local_plugin_count": len(localPlugins)}).Debug("Debug: Checking local plugins for ID")
	for _, plugin := range localPlugins {
		if plugin.Name == pluginName {
			ids = append(ids, plugin.ID)
			Log.WithFields(logrus.Fields{"plugin_name": pluginName, "local_plugin_id": plugin.ID}).Debug("Debug: Found local plugin ID")
		}
	}

	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "possible_ids": ids, "id_count": len(ids)}).Debug("Debug: Final possible IDs list")
	return ids
}

// needsUpdate checks if a plugin needs updating by comparing server version with GitHub latest
func (pm *PluginManager) needsUpdate(pluginName, repo string) (bool, error) {
	
	// First check if plugin is installed
	if !pm.isInstalled(pluginName) {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "github_url": repo}).Debug("Debug: Plugin not installed, needs updating (installing)")
		return true, nil // Not installed, so yes it needs "updating" (installing)
	}

	// Get installed version
	installedVersion, err := pm.getInstalledVersion(pluginName)
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "github_url": repo, "error": err.Error()}).Debug("Debug: Failed to get installed version")
		return false, fmt.Errorf("failed to get installed version: %w", err)
	}

	// Get latest GitHub version
	latestVersion, err := pm.getLatestGitHubVersion(repo)
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "github_url": repo, "local_version": installedVersion, "error": err.Error()}).Debug("Debug: Failed to get latest GitHub version")
		return false, fmt.Errorf("failed to get latest GitHub version: %w", err)
	}

	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "github_url": repo, "local_version": installedVersion, "latest_version": latestVersion}).Debug("Debug: Comparing plugin versions")

	// Compare versions
	needsUpdate := pm.isNewerVersion(latestVersion, installedVersion)
	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "github_url": repo, "local_version": installedVersion, "latest_version": latestVersion, "needs_update": needsUpdate}).Debug("Debug: Version comparison result")
	return needsUpdate, nil
}

// getInstalledVersion gets the version of an installed plugin
func (pm *PluginManager) getInstalledVersion(pluginName string) (string, error) {
	
	plugins, _, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": pluginName, "error": err.Error()}).Debug("Debug: Failed to get plugins list")
		return "", err
	}

	// Check by common plugin IDs for this name
	possibleIDs := pm.getPossibleIDs(pluginName)
	allPlugins := append(plugins.Active, plugins.Inactive...)

	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "possible_ids": possibleIDs}).Debug("Debug: Looking for plugin version")

	for _, plugin := range allPlugins {
		for _, id := range possibleIDs {
			if plugin.Id == id {
				Log.WithFields(logrus.Fields{"plugin_name": pluginName, "plugin_id": id, "plugin_version": plugin.Version}).Debug("Debug: Found plugin version")
				return plugin.Version, nil
			}
		}
	}
	
	Log.WithFields(logrus.Fields{"plugin_name": pluginName, "possible_ids": possibleIDs}).Debug("Debug: Plugin not found for version check")
	return "", fmt.Errorf("plugin %s not found", pluginName)
}

// getLatestGitHubVersion gets the latest release version from GitHub
func (pm *PluginManager) getLatestGitHubVersion(repo string) (string, error) {
	
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	Log.WithFields(logrus.Fields{"github_url": repo, "api_url": url}).Debug("Debug: Fetching latest GitHub version")
	
	resp, err := http.Get(url)
	if err != nil {
		Log.WithFields(logrus.Fields{"github_url": repo, "api_url": url, "error": err.Error()}).Debug("Debug: Failed to fetch GitHub release")
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		Log.WithFields(logrus.Fields{"github_url": repo, "api_url": url, "error": err.Error()}).Debug("Debug: Failed to decode GitHub release response")
		return "", err
	}

	Log.WithFields(logrus.Fields{"github_url": repo, "latest_version": release.TagName}).Debug("Debug: Successfully got latest GitHub version")
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
	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "api_url": url}).Debug("Debug: Getting latest release info")
	
	resp, err := http.Get(url)
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "api_url": url, "error": err.Error()}).Debug("Debug: Failed to get release info")
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
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "error": err.Error()}).Debug("Debug: Failed to decode release response")
		return err
	}

	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_count": len(release.Assets)}).Debug("Debug: Got release info, looking for .tar.gz asset")

	// Find .tar.gz asset
	var downloadURL string
	var assetName string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".tar.gz") && !strings.Contains(asset.Name, "linux") &&
			!strings.Contains(asset.Name, "darwin") && !strings.Contains(asset.Name, "windows") {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}

	if downloadURL == "" {
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_count": len(release.Assets)}).Debug("Debug: No suitable .tar.gz found in assets")
		return fmt.Errorf("no suitable .tar.gz found")
	}

	filename := plugin.PluginID + "-" + release.TagName + ".tar.gz"
	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_name": assetName, "download_url": downloadURL, "filename": filename}).Debug("Debug: Found suitable asset, downloading")

	// Download to plugins directory
	return pm.downloadFile(downloadURL, filename)
}

// downloadFile downloads a file to the plugins directory
func (pm *PluginManager) downloadFile(url, filename string) error {
	
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	_ = os.MkdirAll(pluginsDir, 0755)
	filePath := filepath.Join(pluginsDir, filename)

	Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "plugins_dir": pluginsDir, "file_path": filePath}).Debug("Debug: Starting file download")

	resp, err := http.Get(url)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "error": err.Error()}).Debug("Debug: Failed to download file")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	file, err := os.Create(filePath)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath, "error": err.Error()}).Debug("Debug: Failed to create file")
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath, "error": err.Error()}).Debug("Debug: Failed to copy file contents")
	} else {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath}).Debug("Debug: Successfully downloaded file")
	}
	return err
}

// cleanPlugin cleans a plugin build directory
func (pm *PluginManager) cleanPlugin(pluginPath string) error {
	
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Debug: Plugin directory not found for cleaning")
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Debug: Running make clean")
	cmd := exec.Command("make", "clean")
	cmd.Dir = pluginPath
	err := cmd.Run()
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Debug: Make clean failed")
	} else {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Debug: Make clean successful")
	}
	return err
}

// buildPlugin builds a plugin from source
func (pm *PluginManager) buildPlugin(pluginPath string) error {
	
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Debug: Plugin directory not found for building")
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Debug: Running make dist")
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	err := cmd.Run()
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Debug: Make dist failed")
	} else {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Debug: Make dist successful")
	}
	return err
}

// uploadPlugin uploads and enables a plugin
func (pm *PluginManager) uploadPlugin(bundlePath string) error {
	
	file, err := os.Open(bundlePath)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "error": err.Error()}).Debug("Debug: Failed to open plugin bundle")
		return err
	}
	defer func() { _ = file.Close() }()

	Log.WithFields(logrus.Fields{"bundle_path": bundlePath}).Debug("Debug: Uploading plugin bundle")
	manifest, _, err := pm.client.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "error": err.Error()}).Debug("Debug: Failed to upload plugin")
		return err
	}

	Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "plugin_version": manifest.Version}).Debug("Debug: Plugin uploaded, enabling")
	_, err = pm.client.API.EnablePlugin(context.Background(), manifest.Id)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "error": err.Error()}).Debug("Debug: Failed to enable plugin")
	} else {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "plugin_version": manifest.Version}).Debug("Debug: Plugin enabled successfully")
	}
	return err
}
