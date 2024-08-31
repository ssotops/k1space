package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	clusterTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true).
			Padding(0, 1)

	filePathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5F9F9F")).
			Italic(true)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(1).
			Width(100)

	configStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF69B4")).
			Padding(1).
			Width(100)
)

func provisionCluster() {
	log.Info("Starting provisionCluster function")
	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the index.hcl file exists and is correctly formatted.")
		return
	}

	log.Info("Index file loaded successfully", "version", indexFile.Version, "lastUpdated", indexFile.LastUpdated)
	log.Info("Configs found", "count", len(indexFile.Configs))

	// List available configs
	var selectedConfig string
	configOptions := make([]huh.Option[string], 0, len(indexFile.Configs))
	for config, details := range indexFile.Configs {
		log.Info("Config found", "name", config, "fileCount", len(details.Files))
		configOptions = append(configOptions, huh.NewOption(config, config))
	}

	if len(configOptions) == 0 {
		log.Warn("No configurations found in the index file")
		fmt.Println("No configurations available. Please create a configuration first.")
		fmt.Println("You can create a configuration using the 'Config' -> 'Create Config' option in the main menu.")
		return
	}

	log.Info("Presenting config selection to user", "optionCount", len(configOptions))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a configuration").
				Options(configOptions...).
				Value(&selectedConfig),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in config selection", "error", err)
		return
	}
	log.Info("User selected config", "selectedConfig", selectedConfig)

	// Get files for the selected config
	files := indexFile.Configs[selectedConfig].Files

	// Prepare the content for the TUI
	var configContent strings.Builder
	var fileContents []string
	var filePaths []string

	configContent.WriteString(fmt.Sprintf("Configuration: %s\n", selectedConfig))
	configContent.WriteString(fmt.Sprintf("File count: %d\n", len(files)))

	for _, file := range files {
		cleanFile := strings.Trim(file, "\"")
		content, err := os.ReadFile(filepath.Clean(cleanFile))
		if err != nil {
			log.Error("Error reading file", "file", cleanFile, "error", err)
			continue
		}
		fileContents = append(fileContents, string(content))
		filePaths = append(filePaths, cleanFile)
	}

	// Create the TUI
	var sb strings.Builder

	// Config summary
	sb.WriteString(configStyle.Render(clusterTitleStyle.Render("Configuration Summary") + "\n" + configContent.String()))
	sb.WriteString("\n\n")

	// File contents
	for i, content := range fileContents {
		fileName := filepath.Base(filePaths[i])
		sb.WriteString(contentStyle.Render(
			clusterTitleStyle.Render(fileName) + "\n" +
				filePathStyle.Render(filePaths[i]) + "\n\n" +
				content,
		))
		sb.WriteString("\n\n")
	}

	// Print the TUI
	fmt.Println(sb.String())

	// Confirmation to provision
	var confirmProvision bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you want to proceed with provisioning the cluster?").
				Value(&confirmProvision),
		),
	)

	err = confirmForm.Run()
	if err != nil {
		log.Error("Error in confirmation prompt", "error", err)
		return
	}

	if confirmProvision {
		log.Info("User confirmed cluster provisioning")
		fmt.Println("Provisioning cluster...")
		// Add logic here to run 00-init.sh
		// This is where you would implement the actual cluster provisioning
		// For example:
		// err := runInitScript(filepath.Join(filepath.Dir(filePaths[0]), "00-init.sh"))
		// if err != nil {
		//     log.Error("Error provisioning cluster", "error", err)
		//     return
		// }
		// fmt.Println("Cluster provisioned successfully!")
	} else {
		log.Info("User cancelled cluster provisioning")
		fmt.Println("Cluster provisioning cancelled.")
	}
}

// Add any additional cluster-related functions here
// For example:
// func deleteCluster() { ... }
// func listClusters() { ... }
// func updateCluster() { ... }
