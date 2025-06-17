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
