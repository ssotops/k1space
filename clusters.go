package main

import (
  "bufio"
  "io"
	"fmt"
	"os"
  "os/exec"
	"path/filepath"
	"strings"
  "time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
)

func provisionCluster() {
	log.Info("Starting provisionCluster function")
	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the config.hcl file exists and is correctly formatted.")
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

	// Render the TUI using the function from dashboard.go
	tuiContent := renderClusterProvisioningTUI(selectedConfig, configContent.String(), fileContents, filePaths)
	fmt.Println(tuiContent)

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

		// Find the 00-init.sh file
		var initScriptPath string
		for _, file := range filePaths {
			if strings.HasSuffix(file, "00-init.sh") {
				initScriptPath = file
				break
			}
		}

		if initScriptPath == "" {
			log.Error("00-init.sh not found in the configuration files")
			fmt.Println("Error: 00-init.sh not found. Cannot provision cluster.")
			return
		}

		// Extract cloud, region, and prefix from the selected config
		parts := strings.Split(selectedConfig, "_")
		if len(parts) != 3 {
			log.Error("Invalid config name format", "config", selectedConfig)
			fmt.Println("Error: Invalid configuration name format. Cannot provision cluster.")
			return
		}
		cloud, region, prefix := parts[0], parts[1], parts[2]

		// Run the provisioning script
		err := runProvisioningScript(initScriptPath, cloud, region, prefix)
		if err != nil {
			log.Error("Error provisioning cluster", "error", err)
			fmt.Println("Error provisioning cluster:", err)
		} else {
			fmt.Println("Cluster provisioning completed successfully!")
		}
	} else {
		log.Info("User cancelled cluster provisioning")
		fmt.Println("Cluster provisioning cancelled.")
	}
}

func runProvisioningScript(scriptPath, cloud, region, prefix string) error {
	// Create log directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting home directory: %w", err)
	}
	logDir := filepath.Join(homeDir, ".ssot", "k1space", ".logs", cloud, region, prefix)
	err = os.MkdirAll(logDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating log directory: %w", err)
	}

	// Create log file
	timestamp := time.Now().Format("20060102-150405")
	logFileName := fmt.Sprintf("00-init-%s.log", timestamp)
	logFilePath := filepath.Join(logDir, logFileName)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("error creating log file: %w", err)
	}
	defer logFile.Close()

	// Prepare command
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = filepath.Dir(scriptPath)

	// Set up pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("error starting script: %w", err)
	}

	// Create a channel to signal when we're done reading output
	done := make(chan bool)

	// Function to read from a pipe and write to both console and log file
	readAndLog := func(pipe io.Reader, prefix string) {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(prefix, line)
			logFile.WriteString(prefix + line + "\n")
		}
		done <- true
	}

	// Start goroutines to read stdout and stderr
	go readAndLog(stdout, "")
	go readAndLog(stderr, "ERROR: ")

	// Wait for both stdout and stderr to be fully read
	<-done
	<-done

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("error running script: %w", err)
	}

	return nil
}

func deprovisionCluster() {
    log.Info("Starting deprovisionCluster function")

    // Load index file to get available configurations
    indexFile, err := loadIndexFile()
    if err != nil {
        log.Error("Error loading index file", "error", err)
        fmt.Println("Failed to load configurations. Please ensure that the index.hcl file exists and is correctly formatted.")
        return
    }

    // Prompt user to select a cluster to deprovision
    var selectedConfig string
    configOptions := make([]huh.Option[string], 0, len(indexFile.Configs))
    for config := range indexFile.Configs {
        configOptions = append(configOptions, huh.NewOption(config, config))
    }

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewSelect[string]().
                Title("Select a cluster to deprovision").
                Options(configOptions...).
                Value(&selectedConfig),
        ),
    )

    err = form.Run()
    if err != nil {
        log.Error("Error in config selection", "error", err)
        return
    }

    // Extract cloud, region, and prefix from the selected config
    parts := strings.Split(selectedConfig, "_")
    if len(parts) != 3 {
        log.Error("Invalid config name format", "config", selectedConfig)
        fmt.Println("Invalid configuration name format. Deprovisioning cancelled.")
        return
    }
    cloud, region, prefix := parts[0], parts[1], parts[2]

    // Generate deprovisioning script
    scriptContent := generateDeprovisionScript(cloud, region, prefix)

    // Write the script to a file
    scriptPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", cloud, region, prefix, "deprovision.sh")
    err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
    if err != nil {
        log.Error("Error writing deprovision script", "error", err)
        return
    }

    fmt.Printf("Deprovisioning script generated at: %s\n", scriptPath)
    fmt.Println("Please review the script and run it manually to deprovision the cluster.")
}

func generateDeprovisionScript(cloud, region, prefix string) string {
    return fmt.Sprintf(`#!/bin/bash
set -e

# Install dependencies
brew install terraform kubectl gum

# Get kubeconfig
kubectl config use-context %s-%s-%s

# Get Vault token
VAULT_TOKEN=$(kubectl -n vault get secrets/vault-unseal-secret --template='{{index .data "root-token"}}' | base64 -d)

# Set environment variables
kubefirst terraform set-env \
  --vault-token $VAULT_TOKEN \
  --vault-url https://vault.%s.%s.cloud \
  --output-file .env
source .env

# Clone gitops repository
git clone git@github.com:<my-org>/gitops.git
cd gitops/terraform

# Deprovision cloud provider resources
cd %s
terraform init
terraform destroy

# Deprovision git provider resources
cd ../github  # or ../gitlab for GitLab
terraform init
terraform destroy

# Clean up local resources
cd ../../..
rm -rf gitops
rm .env

# Remove k3d cluster
kubefirst launch down

echo "Deprovisioning complete. Please manually remove any remaining cloud resources if necessary."
`, cloud, region, prefix, region, cloud, cloud)
}
