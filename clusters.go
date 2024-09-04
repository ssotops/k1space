package main

import (
	"bufio"
	"fmt"
	"io"
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

	// Check if the K1_CONSOLE_REMOTE_URL environment variable is set; re: kubefirst.dev issue
	value := os.Getenv("K1_CONSOLE_REMOTE_URL")
	log.Info("K1_CONSOLE_REMOTE_URL:", value)

	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the config.hcl file exists and is correctly formatted.")
		return
	}

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

	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the config.hcl file exists and is correctly formatted.")
		return
	}

	if len(indexFile.Configs) == 0 {
		fmt.Println("No clusters found to deprovision.")
		return
	}

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

	parts := strings.Split(selectedConfig, "_")
	if len(parts) != 3 {
		log.Error("Invalid config name format", "config", selectedConfig)
		fmt.Println("Invalid configuration name format. Deprovisioning cancelled.")
		return
	}
	cloud, region, prefix := parts[0], parts[1], parts[2]

	scriptPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", cloud, region, prefix, "deprovision.sh")

	regenerate := false
	if _, err := os.Stat(scriptPath); err == nil {
		regenerateForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("A deprovision script already exists. Do you want to regenerate it?").
					Value(&regenerate),
			),
		)

		err = regenerateForm.Run()
		if err != nil {
			log.Error("Error in regenerate confirmation", "error", err)
			return
		}

		if !regenerate {
			fmt.Println("Using existing deprovision script.")
		} else {
			fmt.Println("Regenerating deprovision script.")
		}
	}

	if _, err := os.Stat(scriptPath); os.IsNotExist(err) || regenerate {
		scriptContent := generateDeprovisionScript(cloud, region, prefix)
		if scriptContent == "" {
			fmt.Println("Failed to generate deprovisioning script. Please check the logs for more information.")
			return
		}

		err = os.WriteFile(scriptPath, []byte(scriptContent), 0755)
		if err != nil {
			log.Error("Error writing deprovision script", "error", err)
			return
		}

		fmt.Printf("Deprovisioning script generated at: %s\n", scriptPath)
	}

	fmt.Println("Please review the script before running it to deprovision the cluster.")

	var runScript bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you want to run the deprovisioning script now?").
				Value(&runScript),
		),
	)

	err = confirmForm.Run()
	if err != nil {
		log.Error("Error in run script confirmation", "error", err)
		return
	}

	if runScript {
		cmd := exec.Command("bash", scriptPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			log.Error("Error running deprovision script", "error", err)
			fmt.Println("Deprovisioning script encountered an error. Please check the output and try running it manually if necessary.")
		} else {
			fmt.Println("Deprovisioning script completed successfully.")
		}
	} else {
		fmt.Println("Deprovisioning script not run. You can run it manually later.")
	}
}

func generateDeprovisionScript(cloud, region, prefix string) string {
	// Load the .local.cloud.env file
	envFilePath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", cloud, region, prefix, ".local.cloud.env")
	envContent, err := os.ReadFile(envFilePath)
	if err != nil {
		log.Error("Error reading .local.cloud.env file", "error", err)
		return ""
	}

	// Parse the environment variables
	envVars := make(map[string]string)
	for _, line := range strings.Split(string(envContent), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			key := strings.TrimPrefix(parts[0], "export ")
			value := strings.Trim(parts[1], "\"")
			envVars[key] = value
		}
	}

	// Extract required values
	clusterName := envVars[fmt.Sprintf("K2_%s_%s_CLUSTER_NAME", strings.ToUpper(cloud), strings.ToUpper(region))]
	gitProvider := envVars[fmt.Sprintf("K2_%s_%s_GIT_PROVIDER", strings.ToUpper(cloud), strings.ToUpper(region))]
	gitOrg := envVars[fmt.Sprintf("K2_%s_%s_%s_ORG", strings.ToUpper(cloud), strings.ToUpper(region), strings.ToUpper(gitProvider))]
	domain := envVars[fmt.Sprintf("K2_%s_%s_DOMAIN_NAME", strings.ToUpper(cloud), strings.ToUpper(region))]
	subdomain := envVars[fmt.Sprintf("K2_%s_%s_SUBDOMAIN", strings.ToUpper(cloud), strings.ToUpper(region))]

	return fmt.Sprintf(`#!/bin/bash
set -e

echo "Deprovisioning cluster for %s in region %s with prefix %s"

# Check for required tools
for cmd in kubectl kubefirst terraform doctl; do
    if ! command -v $cmd &> /dev/null; then
        echo "Error: $cmd is not installed or not in PATH"
        exit 1
    fi
done

# Get kubeconfig
CLUSTER_NAME="%s"
doctl kubernetes cluster kubeconfig save $CLUSTER_NAME

# Get the actual context name from kubectl
CONTEXT_NAME=$(kubectl config get-contexts --output=name | grep $CLUSTER_NAME)

if [ -z "$CONTEXT_NAME" ]; then
    echo "Error: Unable to find context for cluster $CLUSTER_NAME"
    exit 1
fi

# Use the found context
kubectl config use-context $CONTEXT_NAME

# Get Vault token
VAULT_TOKEN=$(kubectl --context $CONTEXT_NAME -n vault get secrets/vault-unseal-secret --template='{{index .data "root-token"}}' | base64 -d)
if [ -z "$VAULT_TOKEN" ]; then
    echo "Error: Failed to retrieve Vault token"
    exit 1
fi

# Set environment variables
kubefirst terraform set-env \
  --vault-token $VAULT_TOKEN \
  --vault-url https://vault.%s.%s \
  --output-file .env
source .env

# Clone gitops repository
REPO_PATH=~/.ssot/k1space/%s/%s/%s/.repositories/gitops
git clone git@%s.com:%s/gitops.git $REPO_PATH
ln -sf $REPO_PATH ~/.ssot/k1space/%s/%s/%s/gitops
cd $REPO_PATH/terraform

# Deprovision cloud provider resources
cd %s
terraform init
terraform destroy -auto-approve

# Deprovision git provider resources
cd ../%s
terraform init
terraform destroy -auto-approve

# Remove k3d cluster
kubefirst launch down

# Cleanup
cd ~
rm -rf $REPO_PATH ~/.ssot/k1space/%s/%s/%s/gitops .env

echo "Deprovisioning complete. Please manually remove any remaining cloud resources if necessary."
`, cloud, region, prefix, clusterName, subdomain, domain, cloud, region, prefix, gitProvider, gitOrg, cloud, region, prefix, cloud, gitProvider, cloud, region, prefix)
}
