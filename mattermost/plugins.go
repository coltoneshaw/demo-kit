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

	"github.com/mattermost/mattermost/server/public/model"
)

// PluginManager handles all plugin-related operations
type PluginManager struct {
	client *Client
}

// NewPluginManager creates a new PluginManager instance
func NewPluginManager(client *Client) *PluginManager {
	return &PluginManager{client: client}
}


// GetPluginInfo returns information about required plugins
func (pm *PluginManager) GetPluginInfo() []PluginInfo {
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

// GetInstalledPlugins retrieves all installed plugins from the server
func (pm *PluginManager) GetInstalledPlugins() (*model.PluginsResponse, error) {
	plugins, resp, err := pm.client.API.GetPlugins(context.Background())
	if err != nil {
		return nil, handleAPIError("failed to get plugins", err, resp)
	}
	return plugins, nil
}

// IsPluginInstalled checks if a plugin is installed on the server
func (pm *PluginManager) IsPluginInstalled(pluginID string) (bool, error) {
	plugins, err := pm.GetInstalledPlugins()
	if err != nil {
		return false, err
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
func (pm *PluginManager) IsPluginBuilt(pluginPath string) bool {
	bundlePath, err := pm.FindPluginBundle(pluginPath)
	if err != nil {
		return false
	}
	// Check if the bundle file actually exists
	_, err = os.Stat(bundlePath)
	return err == nil
}

// BuildPlugin builds a plugin from its source directory using make
func (pm *PluginManager) BuildPlugin(pluginPath string) error {
	// Check if the plugin directory exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory does not exist: %s", pluginPath)
	}

	// Check if Makefile exists
	makefilePath := filepath.Join(pluginPath, "Makefile")
	if _, err := os.Stat(makefilePath); os.IsNotExist(err) {
		return fmt.Errorf("makefile not found in plugin directory: %s", pluginPath)
	}

	fmt.Printf("Building plugin in %s...\n", pluginPath)

	// Run make dist to build the plugin
	cmd := exec.Command("make", "dist")
	cmd.Dir = pluginPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build plugin: %w", err)
	}

	fmt.Printf("✅ Plugin built successfully\n")
	return nil
}

// FindPluginBundle finds the built plugin bundle (.tar.gz) in the dist directory
func (pm *PluginManager) FindPluginBundle(pluginPath string) (string, error) {
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
func (pm *PluginManager) UploadPlugin(bundlePath string) error {
	fmt.Printf("Uploading plugin bundle: %s\n", bundlePath)

	// Open the bundle file
	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to open plugin bundle: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close plugin bundle file: %v\n", closeErr)
		}
	}()

	fmt.Printf("Uploading with force flag (will overwrite existing plugin)\n")
	// Reset file position
	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return fmt.Errorf("❌ failed to reset file position: %w", seekErr)
	}
	manifest, resp, err := pm.client.API.UploadPluginForced(context.Background(), file)
	if err != nil {
		return handleAPIError(fmt.Sprintf("failed to upload plugin bundle '%s': %v", bundlePath, err), err, resp)
	}
	fmt.Printf("✅ Plugin '%s' (ID: %s) uploaded successfully (forced)\n", manifest.Name, manifest.Id)

	// Enable the plugin
	enableResp, enableErr := pm.client.API.EnablePlugin(context.Background(), manifest.Id)
	if enableErr != nil {
		return handleAPIError("failed to enable plugin", enableErr, enableResp)
	}

	fmt.Printf("✅ Plugin '%s' enabled successfully\n", manifest.Name)
	return nil
}

// SetupPlugin sets up a single plugin (build, upload, enable)
func (pm *PluginManager) SetupPlugin(plugin PluginInfo) error {
	fmt.Printf("Checking plugin '%s' (ID: %s)\n", plugin.Name, plugin.ID)

	// Check if plugin is already installed
	installed, err := pm.IsPluginInstalled(plugin.ID)
	if err != nil {
		return fmt.Errorf("failed to check if plugin '%s' is installed: %w", plugin.Name, err)
	}

	if installed {
		fmt.Printf("✅ Plugin '%s' is already installed\n", plugin.Name)
		return nil
	}

	fmt.Printf("Plugin '%s' not found, checking for existing build...\n", plugin.Name)

	// Check if plugin is already built
	if pm.IsPluginBuilt(plugin.Path) {
		fmt.Printf("✅ Plugin '%s' is already built, attempting upload with force...\n", plugin.Name)
	} else {
		fmt.Printf("Building plugin '%s'...\n", plugin.Name)
		// Build the plugin
		if err := pm.BuildPlugin(plugin.Path); err != nil {
			return fmt.Errorf("failed to build plugin '%s': %w", plugin.Name, err)
		}
	}

	// Find the built bundle
	bundlePath, err := pm.FindPluginBundle(plugin.Path)
	if err != nil {
		return fmt.Errorf("failed to find plugin bundle for '%s': %w", plugin.Name, err)
	}

	if err := pm.UploadPlugin(bundlePath); err != nil {
		return fmt.Errorf("failed to upload plugin '%s': %w", plugin.Name, err)
	}

	fmt.Printf("✅ Plugin '%s' setup completed\n", plugin.Name)
	return nil
}

// SetupAllPlugins ensures all required plugins are installed and enabled
func (pm *PluginManager) SetupAllPlugins() error {
	fmt.Println("Setting up plugins...")

	plugins := pm.GetPluginInfo()

	for _, plugin := range plugins {
		if err := pm.SetupPlugin(plugin); err != nil {
			fmt.Printf("❌ Failed to setup plugin '%s': %v\n", plugin.Name, err)
			// Continue with other plugins instead of failing completely
			continue
		}
	}

	return nil
}

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetPluginDownloadInfo returns plugin configurations from the config file
func (pm *PluginManager) GetPluginDownloadInfo(config *Config) []PluginConfig {
	if config == nil || len(config.Plugins) == 0 {
		return []PluginConfig{}
	}
	return config.Plugins
}

// GetLatestRelease fetches the latest release from a GitHub repository
func (pm *PluginManager) GetLatestRelease(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	fmt.Printf("Fetching GitHub release from: %s\n", url)
	
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release for %s: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d for %s (URL: %s)", resp.StatusCode, repo, url)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub release response for %s: %w", repo, err)
	}

	return &release, nil
}

// GetPluginsDirectory returns the path to the plugins directory
func (pm *PluginManager) GetPluginsDirectory() string {
	// Try multiple possible paths
	possiblePaths := []string{
		"../files/mattermost/plugins",           // From mattermost subdirectory
		"files/mattermost/plugins",              // From root directory
		"../../files/mattermost/plugins",       // From deeper subdirectory
		"/Users/coltonshaw/development/demo-kit/files/mattermost/plugins", // Absolute fallback
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	// If none exist, return the most likely path
	return "../files/mattermost/plugins"
}

// DownloadPluginFromGitHub downloads a plugin from GitHub releases to the plugins directory
func (pm *PluginManager) DownloadPluginFromGitHub(pluginInfo PluginConfig) error {
	fmt.Printf("Checking %s plugin...\n", pluginInfo.Name)

	// Get the latest release
	release, err := pm.GetLatestRelease(pluginInfo.Repo)
	if err != nil {
		return fmt.Errorf("failed to get latest release for %s: %w", pluginInfo.Name, err)
	}

	// Find the plugin bundle in the release assets
	// Only download generic .tar.gz files (not platform-specific ones)
	var downloadURL string
	var assetName string
	
	// Look for generic plugin files (not platform-specific)
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".tar.gz") && 
		   !strings.Contains(asset.Name, "linux") && 
		   !strings.Contains(asset.Name, "darwin") && 
		   !strings.Contains(asset.Name, "windows") {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no .tar.gz plugin bundle found for %s in release %s", pluginInfo.Name, release.TagName)
	}

	// Create plugins directory if it doesn't exist
	pluginsDir := pm.GetPluginsDirectory()
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	// Check if the file already exists
	filePath := filepath.Join(pluginsDir, assetName)
	if _, err := os.Stat(filePath); err == nil {
		fmt.Printf("✅ %s plugin %s already downloaded\n", pluginInfo.Name, release.TagName)
		return nil
	}

	fmt.Printf("📦 Downloading %s plugin %s...\n", pluginInfo.Name, release.TagName)

	// Download the plugin
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download %s plugin: %w", pluginInfo.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create plugin file %s: %w", filePath, err)
	}
	defer func() { _ = file.Close() }()

	// Copy downloaded content to file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save %s plugin file: %w", pluginInfo.Name, err)
	}

	fmt.Printf("✅ Downloaded %s plugin %s to %s\n", pluginInfo.Name, release.TagName, filePath)
	return nil
}

// DownloadLatestPlugins downloads the latest versions of plugins specified in config
func (pm *PluginManager) DownloadLatestPlugins(config *Config) error {
	plugins := pm.GetPluginDownloadInfo(config)
	
	if len(plugins) == 0 {
		fmt.Println("No plugins configured for download from GitHub")
		return nil
	}

	fmt.Println("Downloading latest plugins from GitHub...")

	for _, plugin := range plugins {
		if err := pm.DownloadPluginFromGitHub(plugin); err != nil {
			fmt.Printf("❌ Failed to download %s plugin: %v\n", plugin.Name, err)
			// Continue with other plugins instead of failing completely
			continue
		}
	}

	return nil
}

// InstallPluginsFromDirectory installs all plugins from the plugins directory
func (pm *PluginManager) InstallPluginsFromDirectory() error {
	pluginsDir := pm.GetPluginsDirectory()
	
	// Debug: show current working directory and plugin path
	if cwd, err := os.Getwd(); err == nil {
		fmt.Printf("Current working directory: %s\n", cwd)
	}
	fmt.Printf("Looking for plugins in: %s\n", pluginsDir)
	
	// Check if directory exists
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		fmt.Printf("⚠️ Plugins directory %s does not exist, skipping plugin installation\n", pluginsDir)
		return nil
	}

	// Read directory contents
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	fmt.Printf("Installing plugins from %s...\n", pluginsDir)

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".tar.gz") {
			continue
		}

		pluginPath := filepath.Join(pluginsDir, file.Name())
		fmt.Printf("📦 Installing plugin: %s\n", file.Name())

		// Upload the plugin
		if err := pm.UploadPlugin(pluginPath); err != nil {
			fmt.Printf("❌ Failed to upload plugin %s: %v\n", file.Name(), err)
			continue
		}

		fmt.Printf("✅ Successfully installed plugin: %s\n", file.Name())
	}

	return nil
}

// SetupLatestPlugins downloads plugins from config and installs all plugins from directory
func (pm *PluginManager) SetupLatestPlugins(config *Config) error {
	fmt.Println("Setting up latest plugins...")

	// First, download any plugins specified in the config
	if err := pm.DownloadLatestPlugins(config); err != nil {
		return fmt.Errorf("failed to download plugins from config: %w", err)
	}

	// Then, install all plugins found in the plugins directory
	if err := pm.InstallPluginsFromDirectory(); err != nil {
		return fmt.Errorf("failed to install plugins from directory: %w", err)
	}

	return nil
}