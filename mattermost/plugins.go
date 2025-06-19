package mattermost

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

// Helper methods for plugin operations

// isInstalledByID checks if a plugin is installed by its exact plugin ID
func (pm *PluginManager) isInstalledByID(pluginID string) bool {
	plugins, _, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_id": pluginID, "error": err.Error()}).Debug("Failed to get plugins list, assuming not installed")
		return false // Assume not installed if we can't check
	}

	allPlugins := append(plugins.Active, plugins.Inactive...)
	for _, plugin := range allPlugins {
		if plugin.Id == pluginID {
			Log.WithFields(logrus.Fields{"plugin_id": pluginID, "plugin_version": plugin.Version}).Debug("Found installed plugin by ID")
			return true
		}
	}

	Log.WithFields(logrus.Fields{"plugin_id": pluginID}).Debug("Plugin not found by ID")
	return false
}

// downloadPlugin downloads a plugin from GitHub
func (pm *PluginManager) downloadPlugin(plugin PluginConfig) error {

	// Get latest release
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", plugin.Repo)
	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "plugin_id": plugin.PluginID, "api_url": url}).Debug("Getting latest release info")

	resp, err := http.Get(url)
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "api_url": url, "error": err.Error()}).Debug("Failed to get release info")
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
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "error": err.Error()}).Debug("Failed to decode release response")
		return err
	}

	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_count": len(release.Assets)}).Debug("Got release info, looking for .tar.gz asset")

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
		Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_count": len(release.Assets)}).Debug("No suitable .tar.gz found in assets")
		return fmt.Errorf("no suitable .tar.gz found")
	}

	filename := plugin.PluginID + "-" + release.TagName + ".tar.gz"
	Log.WithFields(logrus.Fields{"plugin_name": plugin.Name, "github_url": plugin.Repo, "release_tag": release.TagName, "asset_name": assetName, "download_url": downloadURL, "filename": filename}).Debug("Found suitable asset, downloading")

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

	Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "plugins_dir": pluginsDir, "file_path": filePath}).Debug("Starting file download")

	resp, err := http.Get(url)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "error": err.Error()}).Debug("Failed to download file")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	file, err := os.Create(filePath)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath, "error": err.Error()}).Debug("Failed to create file")
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath, "error": err.Error()}).Debug("Failed to copy file contents")
	} else {
		Log.WithFields(logrus.Fields{"download_url": url, "filename": filename, "file_path": filePath}).Debug("Successfully downloaded file")
	}
	return err
}

// cleanPlugin cleans a plugin build directory
func (pm *PluginManager) cleanPlugin(pluginPath string) error {

	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Plugin directory not found for cleaning")
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Running make clean")
	cmd := exec.Command("make", "clean")
	cmd.Dir = pluginPath
	err := cmd.Run()
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Make clean failed")
	} else {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Make clean successful")
	}
	return err
}

// buildPlugin builds a plugin from source
func (pm *PluginManager) buildPlugin(pluginPath string) error {

	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Plugin directory not found for building")
		return fmt.Errorf("plugin directory not found: %s", pluginPath)
	}

	Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Running make dist")
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	err := cmd.Run()
	if err != nil {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath, "error": err.Error()}).Debug("Make dist failed")
	} else {
		Log.WithFields(logrus.Fields{"plugin_path": pluginPath}).Debug("Make dist successful")
	}
	return err
}

// uploadPlugin uploads and enables a plugin
func (pm *PluginManager) uploadPlugin(bundlePath string) error {

	file, err := os.Open(bundlePath)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "error": err.Error()}).Debug("Failed to open plugin bundle")
		return err
	}
	defer func() { _ = file.Close() }()

	Log.WithFields(logrus.Fields{"bundle_path": bundlePath}).Debug("Uploading plugin bundle")
	manifest, _, err := pm.client.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "error": err.Error()}).Debug("Failed to upload plugin")
		return err
	}

	Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "plugin_version": manifest.Version}).Debug("Plugin uploaded, enabling")
	_, err = pm.client.API.EnablePlugin(context.Background(), manifest.Id)
	if err != nil {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "error": err.Error()}).Debug("Failed to enable plugin")
	} else {
		Log.WithFields(logrus.Fields{"bundle_path": bundlePath, "plugin_id": manifest.Id, "plugin_version": manifest.Version}).Debug("Plugin enabled successfully")
	}
	return err
}

// PluginInfo represents information about a plugin
type PluginInfo struct {
	ID     string
	Name   string
	Path   string
	Built  bool
	Exists bool
}

// GetPluginInfo returns information about required plugins
func (c *Client) GetPluginInfo() []PluginInfo {
	return []PluginInfo{
		{
			ID:   "com.coltoneshaw.weather",
			Name: "Weather Plugin",
			Path: "../apps/weather-plugin",
		},
		{
			ID:   "com.coltoneshaw.flightaware",
			Name: "FlightAware Plugin",
			Path: "../apps/flightaware-plugin",
		},
		{
			ID:   "com.coltoneshaw.missionops",
			Name: "Mission Operations Plugin",
			Path: "../apps/missionops-plugin",
		},
	}
}

// IsPluginInstalled checks if a plugin is installed on the server
func (c *Client) IsPluginInstalled(pluginID string) (bool, error) {
	plugins, resp, err := c.API.GetPlugins(context.Background())
	if err != nil {
		return false, handleAPIError("failed to get plugins", err, resp)
	}

	// Check both active and inactive plugins
	for _, plugin := range plugins.Active {
		if plugin.Id == pluginID {
			return true, nil
		}
	}
	for _, plugin := range plugins.Inactive {
		if plugin.Id == pluginID {
			return true, nil
		}
	}

	return false, nil
}

// IsPluginBuilt checks if a plugin bundle already exists
func (c *Client) IsPluginBuilt(pluginPath string) bool {
	bundlePath, err := c.FindPluginBundle(pluginPath)
	if err != nil {
		return false
	}
	// Check if the bundle file actually exists
	_, err = os.Stat(bundlePath)
	return err == nil
}

// BuildPlugin builds a plugin from its source directory using make
func (c *Client) BuildPlugin(pluginPath string) error {
	// Check if the plugin directory exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory does not exist: %s", pluginPath)
	}

	// Check if Makefile exists
	makefilePath := filepath.Join(pluginPath, "Makefile")
	if _, err := os.Stat(makefilePath); os.IsNotExist(err) {
		return fmt.Errorf("makefile not found in plugin directory: %s", pluginPath)
	}

	// Run make dist to build the plugin
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	return nil
}

// FindPluginBundle finds the built plugin bundle (.tar.gz) in the dist directory
func (c *Client) FindPluginBundle(pluginPath string) (string, error) {
	distPath := filepath.Join(pluginPath, "dist")

	// Check if dist directory exists
	if _, err := os.Stat(distPath); os.IsNotExist(err) {
		return "", fmt.Errorf("dist directory does not exist: %s", distPath)
	}

	// Find .tar.gz files in dist directory
	matches, err := filepath.Glob(filepath.Join(distPath, "*.tar.gz"))
	if err != nil {
		return "", fmt.Errorf("failed to search for plugin bundle: %w", err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no plugin bundle (.tar.gz) found in %s", distPath)
	}

	// Return the first match (should only be one)
	return matches[0], nil
}

// UploadPlugin uploads and installs a plugin to the Mattermost server
func (c *Client) UploadPlugin(bundlePath string) error {
	Log.WithFields(logrus.Fields{
		"bundle_path": bundlePath,
	}).Info("📤 Uploading plugin bundle")

	// Open the bundle file
	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open plugin bundle: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			Log.WithFields(logrus.Fields{
				"error": closeErr.Error(),
				"file":  bundlePath,
			}).Warn("⚠️ Failed to close plugin bundle file")
		}
	}()

	Log.Info("📤 Uploading with force flag (will overwrite existing plugin)")
	// Reset file position
	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return fmt.Errorf("❌ failed to reset file position: %w", seekErr)
	}
	manifest, resp, err := c.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to upload plugin bundle '%s': %v", bundlePath, err), err, resp)
	}
	Log.WithFields(logrus.Fields{
		"plugin_name": manifest.Name,
		"plugin_id":   manifest.Id,
	}).Info("✅ Plugin uploaded successfully (forced)")

	// Enable the plugin
	enableResp, enableErr := c.API.EnablePlugin(context.Background(), manifest.Id)
	if enableErr != nil {
		return handleAPIError("failed to enable plugin", enableErr, enableResp)
	}

	return nil
}

// processGitHubPlugin downloads and installs a GitHub plugin
func (c *Client) processGitHubPlugin(pluginImport PluginImport) error {
	Log.WithFields(logrus.Fields{
		"plugin_name":   pluginImport.Plugin.Name,
		"github_repo":   pluginImport.Plugin.GithubRepo,
		"plugin_id":     pluginImport.Plugin.PluginID,
		"force_install": pluginImport.Plugin.ForceInstall,
	}).Info("📦 Processing plugin " + pluginImport.Plugin.Name)

	// Check if already installed unless forced
	pm := NewPluginManager(c)
	if !pluginImport.Plugin.ForceInstall && pm.isInstalledByID(pluginImport.Plugin.PluginID) {
		Log.WithFields(logrus.Fields{
			"plugin_name": pluginImport.Plugin.Name,
			"plugin_id":   pluginImport.Plugin.PluginID,
		}).Info("⏭️ Skipping " + pluginImport.Plugin.Name + ": already installed")
		return nil
	}

	// Create PluginConfig for compatibility with existing plugin manager
	pluginConfig := PluginConfig{
		Name:     pluginImport.Plugin.Name,
		Repo:     pluginImport.Plugin.GithubRepo,
		PluginID: pluginImport.Plugin.PluginID,
	}

	// Download the plugin
	Log.WithFields(logrus.Fields{
		"plugin_name": pluginImport.Plugin.Name,
		"github_repo": pluginImport.Plugin.GithubRepo,
		"plugin_id":   pluginImport.Plugin.PluginID,
	}).Info("📥 Downloading plugin from GitHub...")

	if err := pm.downloadPlugin(pluginConfig); err != nil {
		return fmt.Errorf("failed to download GitHub plugin: %w", err)
	}

	// Install from plugins directory
	pluginsDir := "../files/mattermost/plugins"
	if _, err := os.Stat("files/mattermost/plugins"); err == nil {
		pluginsDir = "files/mattermost/plugins"
	}

	// Find the downloaded .tar.gz file
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") && strings.Contains(file.Name(), pluginImport.Plugin.PluginID) {
			pluginPath := filepath.Join(pluginsDir, file.Name())
			if err := pm.uploadPlugin(pluginPath); err != nil {
				return fmt.Errorf("failed to install GitHub plugin: %w", err)
			}
			Log.WithFields(logrus.Fields{
				"plugin_name": pluginImport.Plugin.Name,
				"plugin_id":   pluginImport.Plugin.PluginID,
			}).Info("✅ Successfully installed " + pluginImport.Plugin.Name)
			return nil
		}
	}

	return fmt.Errorf("downloaded plugin file not found for %s", pluginImport.Plugin.Name)
}

// processLocalPlugin builds and installs a local plugin
func (c *Client) processLocalPlugin(pluginImport PluginImport) error {
	Log.WithFields(logrus.Fields{
		"plugin_name":   pluginImport.Plugin.Name,
		"plugin_path":   pluginImport.Plugin.Path,
		"plugin_id":     pluginImport.Plugin.PluginID,
		"force_install": pluginImport.Plugin.ForceInstall,
	}).Info("📦 Processing plugin " + pluginImport.Plugin.Name)

	// Check if already installed unless forced
	pm := NewPluginManager(c)
	if !pluginImport.Plugin.ForceInstall && pm.isInstalledByID(pluginImport.Plugin.PluginID) {
		Log.WithFields(logrus.Fields{
			"plugin_name": pluginImport.Plugin.Name,
			"plugin_id":   pluginImport.Plugin.PluginID,
		}).Info("⏭️ Skipping " + pluginImport.Plugin.Name + ": already installed")
		return nil
	}

	// Check if plugin directory exists
	if _, err := os.Stat(pluginImport.Plugin.Path); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory not found: %s", pluginImport.Plugin.Path)
	}

	// Clean if forced install
	if pluginImport.Plugin.ForceInstall {
		Log.WithFields(logrus.Fields{
			"plugin_path": pluginImport.Plugin.Path,
		}).Debug("🧹 Cleaning plugin before rebuild")
		if err := pm.cleanPlugin(pluginImport.Plugin.Path); err != nil {
			Log.WithFields(logrus.Fields{
				"plugin_path": pluginImport.Plugin.Path,
				"error":       err.Error(),
			}).Warn("⚠️ Warning: Failed to clean plugin")
		}
	}

	// Build the plugin
	Log.WithFields(logrus.Fields{
		"plugin_path": pluginImport.Plugin.Path,
	}).Info("🔨 Building plugin...")

	if err := pm.buildPlugin(pluginImport.Plugin.Path); err != nil {
		return fmt.Errorf("failed to build local plugin: %w", err)
	}

	Log.Info("✅ Plugin build completed, looking for built file...")

	// Find the built .tar.gz file in the plugin's dist directory
	distDir := filepath.Join(pluginImport.Plugin.Path, "dist")

	Log.WithFields(logrus.Fields{
		"dist_dir": distDir,
	}).Debug("🔍 Checking plugin dist directory...")

	files, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin dist directory '%s': %w", distDir, err)
	}

	Log.WithFields(logrus.Fields{
		"dist_dir":   distDir,
		"file_count": len(files),
	}).Debug("🔍 Searching for built plugin file in dist directory...")

	// List all .tar.gz files for debugging
	var tarFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") {
			tarFiles = append(tarFiles, file.Name())
		}
	}

	Log.WithFields(logrus.Fields{
		"tar_gz_files": tarFiles,
	}).Debug("📁 Found .tar.gz files in dist directory")

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") {
			pluginPath := filepath.Join(distDir, file.Name())
			Log.WithFields(logrus.Fields{
				"found_file": file.Name(),
				"full_path":  pluginPath,
			}).Info("📦 Found plugin file in dist directory, installing...")

			if err := pm.uploadPlugin(pluginPath); err != nil {
				return fmt.Errorf("failed to install local plugin: %w", err)
			}
			Log.WithFields(logrus.Fields{
				"plugin_name": pluginImport.Plugin.Name,
				"plugin_id":   pluginImport.Plugin.PluginID,
			}).Info("✅ Successfully installed " + pluginImport.Plugin.Name)
			return nil
		}
	}

	return fmt.Errorf("built plugin file not found for %s (no .tar.gz files found in dist directory '%s')", pluginImport.Plugin.Name, distDir)
}

// processPlugins processes plugin entries from bulk import file
func (c *Client) processPlugins(bulkImportPath string, forcePlugins, forceGitHubPlugins bool) error {
	Log.Info("📦 Processing plugins from JSONL")

	file, err := os.Open(bulkImportPath)
	if err != nil {
		return err
	}
	defer closeWithLog(file, "bulk import file")

	var plugins []PluginImport

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var pluginImport PluginImport
		if err := json.Unmarshal([]byte(line), &pluginImport); err != nil {
			continue
		}

		if pluginImport.Type == "plugin" {
			plugins = append(plugins, pluginImport)
		}
	}

	if len(plugins) == 0 {
		Log.Info("📦 No plugins found in JSONL")
		return nil
	}

	Log.WithFields(logrus.Fields{
		"plugin_count": len(plugins),
	}).Info("📦 Found plugins in JSONL")

	// Process plugins in order: GitHub first, then local
	for _, plugin := range plugins {
		if plugin.Plugin.Source == "github" {
			// Apply force flags: forceGitHubPlugins forces all plugins
			pluginCopy := plugin
			if forceGitHubPlugins {
				pluginCopy.Plugin.ForceInstall = true
			}

			if err := c.processGitHubPlugin(pluginCopy); err != nil {
				Log.WithFields(logrus.Fields{
					"plugin_name": plugin.Plugin.Name,
					"error":       err.Error(),
				}).Error("❌ Failed to process GitHub plugin")
				return fmt.Errorf("failed to process GitHub plugin '%s': %w", plugin.Plugin.Name, err)
			}
		}
	}

	for _, plugin := range plugins {
		if plugin.Plugin.Source == "local" {
			// Apply force flags: forceGitHubPlugins forces all plugins, forcePlugins forces local plugins
			pluginCopy := plugin
			if forceGitHubPlugins || forcePlugins {
				pluginCopy.Plugin.ForceInstall = true
			}

			if err := c.processLocalPlugin(pluginCopy); err != nil {
				Log.WithFields(logrus.Fields{
					"plugin_name": plugin.Plugin.Name,
					"error":       err.Error(),
				}).Error("❌ Failed to process local plugin")
				return fmt.Errorf("failed to process local plugin '%s': %w", plugin.Plugin.Name, err)
			}
		}
	}

	return scanner.Err()
}
