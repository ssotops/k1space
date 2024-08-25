package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/civo/civogo"
	"github.com/digitalocean/godo"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

var (
	style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		PaddingTop(1).
		PaddingBottom(1).
		PaddingLeft(4).
		PaddingRight(4)
)

type CloudConfig struct {
	StaticPrefix     string
	CloudPrefix      string
	Region           string
	Flags            map[string]string
	SelectedNodeType string
}

type IndexFile struct {
	Version       int               `hcl:"version"`
	LastUpdated   string            `hcl:"last_updated"`
	Configs       map[string]Config `hcl:"configs"`
	DefaultValues map[string]string `hcl:"default_values"`
}

type Config struct {
	Files []string `hcl:"files"`
}

type CloudsFile struct {
	LastUpdated    string                        `hcl:"last_updated"`
	CloudRegions   map[string][]string           `hcl:"cloud_regions"`
	CloudNodeTypes map[string][]InstanceSizeInfo `hcl:"cloud_node_types"`
}

type InstanceSizeInfo struct {
	Name          string
	CPUCores      int
	RAMMegabytes  int
	DiskGigabytes int
}

var cloudProviders = []string{
	// "Akamai",
	// "AWS",
	"Civo",
	"DigitalOcean",
	// "Google Cloud",
	// "Vultr",
	// "K3s",
	"K3d",
}

var cloudFlags = map[string][]string{
	"Akamai":       {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"AWS":          {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Civo":         {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"DigitalOcean": {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Google Cloud": {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"Vultr":        {"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"},
	"K3s":          {"alerts-email", "cluster-name", "domain-name", "github-org"},
	"K3d":          {"cluster-name", "github-org"},
}

func main() {
	log.SetOutput(os.Stderr)
	printIntro()

	initializeAndCleanup()
	for {
		action := runMainMenu()
		switch action {
		case "Config":
			runConfigMenu()
		case "Kubefirst":
			runKubefirstMenu()
		case "Cluster":
			runClusterMenu()
		case "k1space":
			runK1spaceMenu()
		case "Exit":
			fmt.Println("Exiting k1space. Goodbye!")
			return
		}
	}
}

func runClusterMenu() {
	for {
		var selected string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Cluster Menu").
					Options(
						huh.NewOption("Provision Cluster", "Provision Cluster"),
						huh.NewOption("Back", "Back"),
					).
					Value(&selected),
			),
		)

		err := form.Run()
		if err != nil {
			log.Error("Error running cluster menu", "error", err)
			return
		}

		switch selected {
		case "Provision Cluster":
			provisionCluster()
		case "Back":
			return
		}
	}
}

func runMainMenu() string {
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("K1Space Main Menu").
				Options(
					huh.NewOption("Config", "Config"),
					huh.NewOption("Kubefirst", "Kubefirst"),
					huh.NewOption("Cluster", "Cluster"),
					huh.NewOption("k1space", "k1space"),
					huh.NewOption("Exit", "Exit"),
				).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		log.Error("Error running main menu", "error", err)
		os.Exit(1)
	}

	return selected
}

func runProvisionMenu() string {
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Cluster Menu").
				Options(
					huh.NewOption("Provision Cluster", "Provision Cluster"),
					huh.NewOption("Back", "Back"),
				).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		log.Error("Error running provision menu", "error", err)
		return "Back"
	}

	return selected
}

func runK1spaceMenu() {
	for {
		var selected string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("k1space Menu").
					Options(
						huh.NewOption("Upgrade k1space", "Upgrade k1space"),
						huh.NewOption("Print Config Paths", "Print Config Paths"),
						huh.NewOption("Print Version Info", "Print Version Info"),
						huh.NewOption("Back", "Back"),
					).
					Value(&selected),
			),
		)

		err := form.Run()
		if err != nil {
			log.Error("Error running k1space menu", "error", err)
			return
		}

		switch selected {
		case "Upgrade k1space":
			upgradeK1space(log.Default())
		case "Print Config Paths":
			printConfigPaths(log.Default())
		case "Print Version Info":
			printVersionInfo(log.Default())
		case "Back":
			return
		}

		// Prompt user to continue or return to main menu
		var continueAction bool
		continueForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to perform another k1space action?").
					Value(&continueAction),
			),
		)

		err = continueForm.Run()
		if err != nil {
			log.Error("Error in continue prompt", "error", err)
			return
		}

		if !continueAction {
			return
		}
	}
}

func runKubefirstMenu() {
	for {
		var selected string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Kubefirst Menu").
					Options(
						huh.NewOption("Clone Repositories", "Clone Repositories"),
						huh.NewOption("Sync Repositories", "Sync Repositories"),
						huh.NewOption("Setup Kubefirst", "Setup Kubefirst"),
						huh.NewOption("Back", "Back"),
					).
					Value(&selected),
			),
		)

		err := form.Run()
		if err != nil {
			log.Error("Error running Kubefirst menu", "error", err)
			return
		}

		switch selected {
		case "Clone Repositories":
			setupKubefirstRepositories()
		case "Sync Repositories":
			syncKubefirstRepositories()
		case "Setup Kubefirst":
			handleKubefirstSetup()
		case "Back":
			return
		}

		// Prompt user to continue or return to main menu
		var continueAction bool
		continueForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to perform another Kubefirst action?").
					Value(&continueAction),
			),
		)

		err = continueForm.Run()
		if err != nil {
			log.Error("Error in continue prompt", "error", err)
			return
		}

		if !continueAction {
			return
		}
	}
}

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

	// Display files in selected config
	files := indexFile.Configs[selectedConfig].Files
	var selectedFile string
	fileOptions := make([]huh.Option[string], 0, len(files))
	for _, file := range files {
		// Remove any surrounding quotes and use only the base filename
		cleanFile := strings.Trim(file, "\"")
		baseFile := filepath.Base(cleanFile)
		fileOptions = append(fileOptions, huh.NewOption(baseFile, cleanFile))
	}

	log.Info("Presenting file selection to user", "optionCount", len(fileOptions))
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a file").
				Options(fileOptions...).
				Value(&selectedFile),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error selecting file", "error", err)
		return
	}
	log.Info("User selected file", "selectedFile", selectedFile)

	// Read and display file contents
	content, err := os.ReadFile(filepath.Clean(selectedFile))
	if err != nil {
		log.Error("Error reading file", "error", err)
		return
	}

	fmt.Printf("Contents of %s:\n\n%s\n\n", filepath.Base(selectedFile), string(content))

	// Prompt user to run 00-init.sh
	var confirmRun bool
	form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you want to run 00-init.sh to provision the cluster?").
				Value(&confirmRun),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in confirmation prompt", "error", err)
		return
	}

	if confirmRun {
		// Summarize the provision cluster process
		fmt.Println("\nProvisioning Cluster Summary:")
		fmt.Printf("- Configuration: %s\n", selectedConfig)
		fmt.Printf("- Init Script: %s\n", filepath.Join(filepath.Dir(selectedFile), "00-init.sh"))
		fmt.Printf("- Kubefirst Script: %s\n", filepath.Join(filepath.Dir(selectedFile), "01-kubefirst-cloud.sh"))
		fmt.Printf("- Environment File: %s\n", filepath.Join(filepath.Dir(selectedFile), ".local.cloud.env"))

		// Final confirmation
		var finalConfirm bool
		form = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Do you want to proceed with provisioning the cluster?").
					Value(&finalConfirm),
			),
		)

		err = form.Run()
		if err != nil {
			log.Error("Error in final confirmation prompt", "error", err)
			return
		}

		if finalConfirm {
			log.Info("User confirmed cluster provisioning")
			fmt.Println("Provisioning cluster...")
			// Add logic here to run 00-init.sh
		} else {
			log.Info("User cancelled cluster provisioning")
			fmt.Println("Cluster provisioning cancelled.")
		}
	} else {
		log.Info("User chose not to run 00-init.sh")
		fmt.Println("Cluster provisioning cancelled.")
	}
}

func setupKubefirstRepositories() {
	repos := []string{
		"github.com/kubefirst/kubefirst",
		"github.com/kubefirst/console",
		"github.com/kubefirst/kubefirst-api",
	}

	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")
	err := os.MkdirAll(repoDir, 0755)
	if err != nil {
		log.Error("Error creating repositories directory", "error", err)
		return
	}

	summary := make([][]string, 0, len(repos)+1)
	summary = append(summary, []string{"Repository", "Clone Path", "Symlink Path", "Status"})

	for _, repo := range repos {
		repoName := filepath.Base(repo)
		repoPath := filepath.Join(repoDir, repoName)
		symlinkPath := filepath.Join(baseDir, repoName)

		if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
			// Repository already exists, sync instead
			fmt.Printf("Repository %s already exists. Syncing...\n", repo)
			status := syncRepository(repoPath)
			summary = append(summary, []string{repo, repoPath, symlinkPath, status})
			continue
		}

		fmt.Printf("Cloning %s...\n", repo)

		cmd := exec.Command("git", "clone", "https://"+repo+".git", repoPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error("Error cloning repository", "repo", repo, "error", err, "output", string(output))
			summary = append(summary, []string{repo, repoPath, symlinkPath, "Failed to clone"})
			continue
		}

		err = os.Symlink(repoPath, symlinkPath)
		if err != nil {
			if !os.IsExist(err) {
				log.Error("Error creating symlink", "repo", repo, "error", err)
				summary = append(summary, []string{repo, repoPath, symlinkPath, "Cloned, failed to symlink"})
				continue
			}
			// Symlink already exists, which is fine
		}

		summary = append(summary, []string{repo, repoPath, symlinkPath, "Success"})
		fmt.Printf("Repository %s setup complete\n", repo)
	}

	printSummaryTable(summary)
}

func syncRepository(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("Error syncing repository", "repo", repoPath, "error", err, "output", string(output))
		return "Failed to sync"
	}

	if strings.Contains(string(output), "Already up to date.") {
		return "Up to date"
	}
	return "Updated"
}

func syncKubefirstRepositories() {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	repoDir := filepath.Join(baseDir, ".repositories")

	repos, err := os.ReadDir(repoDir)
	if err != nil {
		log.Error("Error reading repositories directory", "error", err)
		return
	}

	summary := make([][]string, 0, len(repos)+1)
	summary = append(summary, []string{"Repository", "Path", "Status"})

	for _, repo := range repos {
		if !repo.IsDir() {
			continue
		}

		repoPath := filepath.Join(repoDir, repo.Name())
		fmt.Printf("Syncing %s...\n", repo.Name())

		status := syncRepository(repoPath)
		summary = append(summary, []string{repo.Name(), repoPath, status})
		fmt.Printf("Repository %s sync complete\n", repo.Name())
	}

	printSummaryTable(summary)
}

func printSummaryTable(summary [][]string) {
	fmt.Println(style.Render("\nRepository Setup Summary:"))

	colWidths := make([]int, len(summary[0]))
	for _, row := range summary {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	for i, row := range summary {
		for j, cell := range row {
			fmt.Printf("%-*s", colWidths[j]+2, cell)
		}
		fmt.Println()
		if i == 0 {
			for _, width := range colWidths {
				fmt.Print(strings.Repeat("-", width+2))
			}
			fmt.Println()
		}
	}
}

func runConfigMenu() {
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Config Menu").
				Options(
					huh.NewOption("Create Config", "Create Config"),
					huh.NewOption("Back", "Back"),
				).
				Value(&selected),
		),
	)

	err := form.Run()
	if err != nil {
		log.Error("Error running config menu", "error", err)
		return
	}

	switch selected {
	case "Create Config":
		createConfig()
	case "Back":
		return
	}
}

func createConfig() {
	log.Info("Starting createConfig function")

	config := CloudConfig{
		Flags: make(map[string]string),
	}
	log.Info("CloudConfig initialized", "config", fmt.Sprintf("%+v", config))

	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		return
	}
	log.Info("Index file loaded", "indexFile", fmt.Sprintf("%+v", indexFile))

	cloudsFile, err := loadCloudsFile()
	if err != nil {
		log.Error("Error loading clouds file", "error", err)
		return
	}
	log.Info("Clouds file loaded", "cloudsFile", fmt.Sprintf("%+v", cloudsFile))

	kubefirstPath, err := promptKubefirstBinary()
	if err != nil {
		log.Error("Error selecting kubefirst binary", "error", err)
		return
	}

	err = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter static prefix").
				Description("Default is 'K1'").
				Placeholder("K1").
				Value(&config.StaticPrefix),

			huh.NewSelect[string]().
				Title("Select cloud provider").
				Options(getCloudProviderOptions()...).
				Value(&config.CloudPrefix),
		),
	).Run()

	if err != nil {
		log.Error("Error in initial config form", "error", err)
		return
	}

	// If the user didn't enter anything, use the default "K1"
	if config.StaticPrefix == "" {
		config.StaticPrefix = "K1"
	}

	log.Info("Initial form completed", "StaticPrefix", config.StaticPrefix, "CloudPrefix", config.CloudPrefix)

	// Update cloud regions and node types
	if config.CloudPrefix == "DigitalOcean" {
		err = updateDigitalOceanRegions(&cloudsFile)
		if err != nil {
			log.Error("Error updating DigitalOcean regions", "error", err)
			return
		}
		err = updateDigitalOceanNodeTypes(&cloudsFile)
		if err != nil {
			log.Error("Error updating DigitalOcean node types", "error", err)
			return
		}
	} else if config.CloudPrefix == "Civo" {
		err = updateCivoRegions(&cloudsFile)
		if err != nil {
			log.Error("Error updating Civo regions", "error", err)
			return
		}
		err = updateCivoNodeTypes(&cloudsFile)
		if err != nil {
			log.Error("Error updating Civo node types", "error", err)
			return
		}
	}
	log.Info("Cloud provider specific updates completed")

	flags, err := fetchKubefirstFlags(kubefirstPath, config.CloudPrefix)
	if err != nil {
		log.Error("Error fetching kubefirst flags", "error", err)
		return
	}
	log.Info("Flags retrieved for cloud provider", "Flags", flags)

	if len(flags) == 0 {
		log.Error("No flags found for the selected cloud provider")
		return
	}

	flagInputs := make([]struct{ Name, Value string }, 0, len(flags))
	flagGroups := make([]huh.Field, 0, len(flags))

	for flag, description := range flags {
		defaultValue := indexFile.DefaultValues[flag]
		flagInput := struct{ Name, Value string }{Name: flag, Value: defaultValue}
		flagInputs = append(flagInputs, flagInput)

		var field huh.Field
		switch flag {
		case "cloud-region":
			field = huh.NewSelect[string]().
				Title("Select cloud region").
				Description(description).
				Options(getRegionOptions(config.CloudPrefix, cloudsFile)...).
				Value(&flagInput.Value)
		case "node-type":
			field = huh.NewSelect[string]().
				Title("Select node type").
				Description(description).
				Options(getNodeTypeOptions(config.CloudPrefix, cloudsFile)...).
				Value(&flagInput.Value)
		default:
			field = huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Description(description).
				Placeholder(defaultValue).
				Value(&flagInput.Value)
		}

		flagGroups = append(flagGroups, field)
	}

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		log.Error("Error in flag input form", "error", err)
		return
	}

	// Update config.Flags and indexFile.DefaultValues with the collected values
	for _, fi := range flagInputs {
		if config.Flags == nil {
			config.Flags = make(map[string]string)
		}
		config.Flags[fi.Name] = fi.Value
		if indexFile.DefaultValues == nil {
			indexFile.DefaultValues = make(map[string]string)
		}
		indexFile.DefaultValues[fi.Name] = fi.Value

		if fi.Name == "node-type" {
			config.SelectedNodeType = fi.Value
		}
		if fi.Name == "cloud-region" {
			config.Region = fi.Value
		}
	}

	log.Info("Updated config.Flags and indexFile.DefaultValues", "ConfigFlags", config.Flags, "IndexFileDefaultValues", indexFile.DefaultValues)

	err = generateFiles(config, kubefirstPath)
	if err != nil {
		log.Error("Error generating files", "error", err)
		return
	}
	log.Info("Files generated successfully")

	err = updateIndexFile(config, indexFile)
	if err != nil {
		log.Error("Error updating index file", "error", err)
		return
	}
	log.Info("Index file updated successfully")

	err = updateCloudsFile(config, cloudsFile)
	if err != nil {
		log.Error("Error updating clouds file", "error", err)
		return
	}
	log.Info("Clouds file updated successfully")

	// Define baseDir
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))

	// Pretty-print the summary
	fmt.Println(style.Render("✅ Configuration completed successfully! Summary:"))
	fmt.Println()

	fmt.Printf("☁️ Cloud Provider: %s\n", config.CloudPrefix)
	fmt.Printf("🌎 Region: %s\n", config.Region)
	fmt.Printf("💻 Node Type: %s\n", config.SelectedNodeType)

	// Print relevant file paths
	fmt.Println(style.Render("\n📁 Generated Files:"))
	filePrefix := "  "
	fmt.Printf("%sInit Script: %s\n", filePrefix, filepath.Join(baseDir, "00-init.sh"))
	fmt.Printf("%sKubefirst Script: %s\n", filePrefix, filepath.Join(baseDir, "01-kubefirst-cloud.sh"))
	fmt.Printf("%sEnvironment File: %s\n", filePrefix, filepath.Join(baseDir, ".local.cloud.env"))

	// Print command to run the generated init script
	fmt.Println(style.Render("\n🚀 To run the initialization script, use the following command:"))
	fmt.Printf("cd %s && ./00-init.sh\n", baseDir)

	log.Info("createConfig function completed successfully")
}

func getCivoClient() (*civogo.Client, error) {
	token := os.Getenv("CIVO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("CIVO_TOKEN not found in environment. Please set it and try again")
	}
	return civogo.NewClient(token, "")
}

func updateCivoRegions(cloudsFile *CloudsFile) error {
	client, err := getCivoClient()
	if err != nil {
		return err
	}

	regions, err := client.ListRegions()
	if err != nil {
		return err
	}

	var regionCodes []string
	for _, region := range regions {
		regionCodes = append(regionCodes, region.Code)
	}

	cloudsFile.CloudRegions["Civo"] = regionCodes
	return nil
}

func updateCivoNodeTypes(cloudsFile *CloudsFile) error {
	client, err := getCivoClient()
	if err != nil {
		return err
	}

	sizes, err := client.ListInstanceSizes()
	if err != nil {
		return err
	}

	var sizeInfos []InstanceSizeInfo
	for _, size := range sizes {
		sizeInfos = append(sizeInfos, InstanceSizeInfo{
			Name:          size.Name,
			CPUCores:      size.CPUCores,
			RAMMegabytes:  size.RAMMegabytes,
			DiskGigabytes: size.DiskGigabytes,
		})
	}

	cloudsFile.CloudNodeTypes["Civo"] = sizeInfos
	return nil
}

func getCloudProviderOptions() []huh.Option[string] {
	options := make([]huh.Option[string], len(cloudProviders))
	for i, provider := range cloudProviders {
		options[i] = huh.Option[string]{Key: provider, Value: provider}
	}
	return options
}

func getRegionOptions(cloudProvider string, cloudsFile CloudsFile) []huh.Option[string] {
	regions := cloudsFile.CloudRegions[cloudProvider]
	options := make([]huh.Option[string], len(regions))
	for i, region := range regions {
		options[i] = huh.Option[string]{Key: region, Value: region}
	}
	return options
}

func getNodeTypeOptions(cloudProvider string, cloudsFile CloudsFile) []huh.Option[string] {
	nodeTypes := cloudsFile.CloudNodeTypes[cloudProvider]
	options := make([]huh.Option[string], len(nodeTypes))
	for i, nodeType := range nodeTypes {
		displayName := fmt.Sprintf("%s (CPU Cores: %d, RAM: %d MB, Disk: %d GB)",
			nodeType.Name,
			nodeType.CPUCores,
			nodeType.RAMMegabytes,
			nodeType.DiskGigabytes)
		options[i] = huh.Option[string]{
			Key:   nodeType.Name,
			Value: displayName,
		}
	}
	return options
}

func loadIndexFile() (IndexFile, error) {
	indexPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")
	var indexFile IndexFile

	log.Info("Attempting to read index.hcl", "path", indexPath)

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		log.Info("index.hcl does not exist, creating a new one")
		err := createOrUpdateIndexFile(indexPath, IndexFile{
			Version:       1,
			LastUpdated:   time.Now().UTC().Format(time.RFC3339),
			Configs:       make(map[string]Config),
			DefaultValues: make(map[string]string),
		})
		if err != nil {
			return indexFile, fmt.Errorf("error creating index.hcl: %w", err)
		}
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		log.Error("Failed to read index.hcl", "error", err)
		return indexFile, fmt.Errorf("error reading index.hcl: %w", err)
	}
	log.Info("Successfully read index.hcl", "bytes", len(data))

	content := string(data)
	configs := simpleHCLParser(content)

	indexFile.Configs = make(map[string]Config)
	for configName, files := range configs {
		indexFile.Configs[configName] = Config{Files: files}
		log.Info("Parsed config", "name", configName, "fileCount", len(files))
	}

	log.Info("Finished parsing index.hcl", "configCount", len(indexFile.Configs))
	return indexFile, nil
}

func createOrUpdateIndexFile(path string, indexFile IndexFile) error {
	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	rootBody.SetAttributeValue("version", cty.NumberIntVal(int64(indexFile.Version)))
	rootBody.SetAttributeValue("last_updated", cty.StringVal(indexFile.LastUpdated))

	configsBlock := rootBody.AppendNewBlock("configs", nil)
	configsBody := configsBlock.Body()
	for k, v := range indexFile.Configs {
		configBlock := configsBody.AppendNewBlock(k, nil)
		fileValues := make([]cty.Value, len(v.Files))
		for i, file := range v.Files {
			fileValues[i] = cty.StringVal(filepath.ToSlash(file))
		}
		configBlock.Body().SetAttributeValue("files", cty.ListVal(fileValues))
	}

	defaultValuesBlock := rootBody.AppendNewBlock("default_values", nil)
	defaultValuesBody := defaultValuesBlock.Body()
	for k, v := range indexFile.DefaultValues {
		defaultValuesBody.SetAttributeValue(k, cty.StringVal(v))
	}

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("error creating directory for index.hcl: %w", err)
	}

	err = os.WriteFile(path, f.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("error writing index.hcl: %w", err)
	}

	return nil
}

func updateIndexFile(config CloudConfig, indexFile IndexFile) error {
	indexPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")

	// Update LastUpdated
	indexFile.LastUpdated = time.Now().UTC().Format(time.RFC3339)

	// Add or update the new configuration
	key := fmt.Sprintf("%s_%s", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	indexFile.Configs[key] = Config{
		Files: []string{
			filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "00-init.sh"),
			filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "01-kubefirst-cloud.sh"),
			filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), ".local.cloud.env"),
		},
	}

	// Create HCL file
	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	// Write version and last_updated
	rootBody.SetAttributeValue("version", cty.NumberIntVal(int64(indexFile.Version)))
	rootBody.SetAttributeValue("last_updated", cty.StringVal(indexFile.LastUpdated))

	// Write configs
	configsBlock := rootBody.AppendNewBlock("configs", nil)
	configsBody := configsBlock.Body()
	for k, v := range indexFile.Configs {
		configBlock := configsBody.AppendNewBlock(k, nil)
		fileValues := make([]cty.Value, len(v.Files))
		for i, file := range v.Files {
			// Use filepath.ToSlash to ensure consistent forward slashes
			fileValues[i] = cty.StringVal(filepath.ToSlash(file))
		}
		configBlock.Body().SetAttributeValue("files", cty.ListVal(fileValues))
	}

	// Write default_values
	defaultValuesBlock := rootBody.AppendNewBlock("default_values", nil)
	defaultValuesBody := defaultValuesBlock.Body()
	for k, v := range indexFile.DefaultValues {
		defaultValuesBody.SetAttributeValue(k, cty.StringVal(v))
	}

	// Write the updated index file
	err := os.WriteFile(indexPath, f.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func backupIndexFile(indexFile IndexFile) error {
	sourceFile := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")
	cacheDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", ".cache")

	err := os.MkdirAll(cacheDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating cache directory: %w", err)
	}

	backupFileName := fmt.Sprintf("index_%s.hcl", indexFile.LastUpdated)
	backupFile := filepath.Join(cacheDir, backupFileName)

	err = os.Rename(sourceFile, backupFile)
	if err != nil {
		return fmt.Errorf("error backing up index file: %w", err)
	}

	return nil
}

func convertStringSliceToCtyValueSlice(slice []string) []cty.Value {
	values := make([]cty.Value, len(slice))
	for i, s := range slice {
		values[i] = cty.StringVal(s)
	}
	return values
}

func getDigitalOceanSizes() ([]string, error) {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DIGITALOCEAN_TOKEN not found in environment. Please set it and try again")
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	sizes, _, err := client.Sizes.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	var sizeSlugs []string
	for _, size := range sizes {
		sizeSlugs = append(sizeSlugs, size.Slug)
	}

	return sizeSlugs, nil
}

func updateDigitalOceanNodeTypes(cloudsFile *CloudsFile) error {
	sizes, err := getDigitalOceanSizes()
	if err != nil {
		return err
	}

	var sizeInfos []InstanceSizeInfo
	for _, size := range sizes {
		cpuCores, ramMB, diskGB := parseDigitalOceanSize(size)
		sizeInfos = append(sizeInfos, InstanceSizeInfo{
			Name:          size,
			CPUCores:      cpuCores,
			RAMMegabytes:  ramMB,
			DiskGigabytes: diskGB,
		})
	}

	cloudsFile.CloudNodeTypes["DigitalOcean"] = sizeInfos
	return nil
}

func parseDigitalOceanSize(size string) (cpuCores, ramMB, diskGB int) {
	parts := strings.Split(size, "-")
	if len(parts) < 3 {
		return 0, 0, 0
	}

	cpuStr := strings.TrimSuffix(parts[1], "vcpu")
	cpuCores, _ = strconv.Atoi(cpuStr)

	ramStr := strings.TrimSuffix(parts[2], "gb")
	ramGB, _ := strconv.Atoi(ramStr)
	ramMB = ramGB * 1024

	if len(parts) > 3 {
		diskStr := strings.TrimSuffix(parts[3], "gb")
		diskGB, _ = strconv.Atoi(diskStr)
	}

	return cpuCores, ramMB, diskGB
}

func updateDigitalOceanRegions(cloudsFile *CloudsFile) error {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		return fmt.Errorf("DIGITALOCEAN_TOKEN not found in environment. Please set it and try again")
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	opt := &godo.ListOptions{
		Page:    1,
		PerPage: 200,
	}

	regions, _, err := client.Regions.List(ctx, opt)
	if err != nil {
		return err
	}

	var regionSlugs []string
	for _, region := range regions {
		regionSlugs = append(regionSlugs, region.Slug)
	}

	cloudsFile.CloudRegions["DigitalOcean"] = regionSlugs
	return nil
}

func generateFiles(config CloudConfig, kubefirstPath string) error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		return err
	}

	// Generate .local.cloud.env
	envContent := generateEnvContent(config)
	err = os.WriteFile(filepath.Join(baseDir, ".local.cloud.env"), []byte(envContent), 0644)
	if err != nil {
		return err
	}

	// Generate 00-init.sh
	initContent := generateInitContent()
	err = os.WriteFile(filepath.Join(baseDir, "00-init.sh"), []byte(initContent), 0755)
	if err != nil {
		return err
	}

	// Generate 01-kubefirst-cloud.sh
	kubefirstContent := generateKubefirstContent(config, kubefirstPath)
	err = os.WriteFile(filepath.Join(baseDir, "01-kubefirst-cloud.sh"), []byte(kubefirstContent), 0755)
	if err != nil {
		return err
	}

	return nil
}

func generateEnvContent(config CloudConfig) string {
	var content strings.Builder
	for flag, value := range config.Flags {
		content.WriteString(fmt.Sprintf("export %s_%s_%s_%s=\"%s\"\n", config.StaticPrefix, config.CloudPrefix, strings.ToUpper(config.Region), strings.ToUpper(flag), value))
	}
	return content.String()
}

func generateInitContent() string {
	return `#!/bin/bash
op run --env-file="./.local.cloud.env" -- sh ./01-kubefirst-cloud.sh
`
}

func generateKubefirstContent(config CloudConfig, kubefirstPath string) string {
	var content strings.Builder
	content.WriteString("#!/bin/bash\n\n")
	content.WriteString("./prepare/01-check-dependencies.sh\n\n")
	content.WriteString(fmt.Sprintf("%s %s create \\\n", kubefirstPath, strings.ToLower(config.CloudPrefix)))

	for flag, value := range config.Flags {
		if value != "" {
			content.WriteString(fmt.Sprintf("  --%s \"%s\" \\\n", flag, value))
		}
	}

	return content.String()
}

func setupConsoleEnvironment() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	consoleDir := filepath.Join(baseDir, "console")
	envExamplePath := filepath.Join(consoleDir, ".env.example")
	envPath := filepath.Join(consoleDir, ".env")

	// Read .env.example
	content, err := os.ReadFile(envExamplePath)
	if err != nil {
		return fmt.Errorf("error reading .env.example: %w", err)
	}

	// Check if .env already exists
	if _, err := os.Stat(envPath); err == nil {
		var overwrite bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("The .env file in `Console` already exists. Do you want to overwrite it?").
					Value(&overwrite),
			),
		)

		err = form.Run()
		if err != nil {
			return fmt.Errorf("error in user prompt: %w", err)
		}

		if !overwrite {
			fmt.Println("Skipping .env creation.")
			return nil
		}
	}

	// Create .env file
	err = os.WriteFile(envPath, content, 0644)
	if err != nil {
		return fmt.Errorf("error creating .env file: %w", err)
	}

	fmt.Println("Created .env file for Console.")

	// Set K1_LOCAL_DEBUG environment variable
	err = os.Setenv("K1_LOCAL_DEBUG", "true")
	if err != nil {
		return fmt.Errorf("error setting K1_LOCAL_DEBUG: %w", err)
	}

	fmt.Println("Set K1_LOCAL_DEBUG=true")

	return nil
}

func runKubefirstSetup() error {
	// Setup Console Environment
	err := setupConsoleEnvironment()
	if err != nil {
		log.Error("Error setting up Console environment", "error", err)
		return err
	}

	// Setup Kubefirst API
	err = setupKubefirstAPI()
	if err != nil {
		log.Error("Error setting up Kubefirst API", "error", err)
		return err
	}

	// Setup Kubefirst
	err = setupKubefirst()
	if err != nil {
		log.Error("Error setting up Kubefirst", "error", err)
		return err
	}

	return nil
}

func setupKubefirstAPI() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	apiDir := filepath.Join(baseDir, "kubefirst-api")

	// Checkout feature branch
	cmd := exec.Command("git", "-C", apiDir, "checkout", "feat-custom-repo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out feat-custom-repo: %w\nOutput: %s", err, output)
	}

	fmt.Println("Checked out feat-custom-repo branch for Kubefirst API")

	// TODO: Add instructions to run the API locally
	fmt.Println("Please follow the instructions in the Kubefirst API README to set up and run the API locally.")

	return nil
}

func setupKubefirst() error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")
	kubefirstDir := filepath.Join(baseDir, "kubefirst")

	// Set K1_LOCAL_DEBUG environment variable
	err := os.Setenv("K1_LOCAL_DEBUG", "true")
	if err != nil {
		return fmt.Errorf("error setting K1_LOCAL_DEBUG: %w", err)
	}

	fmt.Println("Set K1_LOCAL_DEBUG=true for Kubefirst")

	// Checkout feature branch
	cmd := exec.Command("git", "-C", kubefirstDir, "checkout", "feat-custom-repo")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error checking out feat-custom-repo: %w\nOutput: %s", err, output)
	}

	fmt.Println("Checked out feat-custom-repo branch for Kubefirst")

	// Update go.mod
	apiDir := filepath.Join(baseDir, "kubefirst-api")
	goModPath := filepath.Join(kubefirstDir, "go.mod")

	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("error reading go.mod: %w", err)
	}

	newContent := strings.Replace(string(goModContent),
		"github.com/kubefirst/kubefirst-api v0.1.26",
		fmt.Sprintf("github.com/kubefirst/kubefirst-api v0.1.26 => %s", apiDir),
		1)

	err = os.WriteFile(goModPath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("error updating go.mod: %w", err)
	}

	fmt.Println("Updated go.mod to point to local Kubefirst API repository")

	// TODO: Add instructions to build and run Kubefirst
	fmt.Println("Please build the Kubefirst binary and execute the create command with the necessary parameters.")

	return nil
}

// Add this function to your main menu or where appropriate
func handleKubefirstSetup() {
	err := runKubefirstSetup()
	if err != nil {
		log.Error("Error in Kubefirst setup", "error", err)
	} else {
		fmt.Println("Kubefirst setup completed successfully!")
	}
}

func updateCloudsFile(cloudConfig CloudConfig, cloudsFile CloudsFile) error {
	cloudsPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "clouds.hcl")

	// Update cloud regions
	if _, exists := cloudsFile.CloudRegions[cloudConfig.CloudPrefix]; !exists {
		cloudsFile.CloudRegions[cloudConfig.CloudPrefix] = []string{}
	}
	if !contains(cloudsFile.CloudRegions[cloudConfig.CloudPrefix], cloudConfig.Region) {
		cloudsFile.CloudRegions[cloudConfig.CloudPrefix] = append(
			cloudsFile.CloudRegions[cloudConfig.CloudPrefix],
			cloudConfig.Region,
		)
	}

	// Create HCL file
	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	// Write last_updated
	rootBody.SetAttributeValue("last_updated", cty.StringVal(time.Now().UTC().Format(time.RFC3339)))

	// Write cloud_regions
	cloudRegionsBlock := rootBody.AppendNewBlock("cloud_regions", nil)
	cloudRegionsBody := cloudRegionsBlock.Body()
	for k, v := range cloudsFile.CloudRegions {
		cloudRegionsBody.SetAttributeValue(k, cty.ListVal(convertStringSliceToCtyValueSlice(v)))
	}

	// Write cloud_node_types
	cloudNodeTypesBlock := rootBody.AppendNewBlock("cloud_node_types", nil)
	cloudNodeTypesBody := cloudNodeTypesBlock.Body()
	for k, v := range cloudsFile.CloudNodeTypes {
		nodeTypeValues := make([]cty.Value, len(v))
		for i, nodeType := range v {
			nodeTypeValues[i] = cty.ObjectVal(map[string]cty.Value{
				"name":           cty.StringVal(nodeType.Name),
				"cpu_cores":      cty.NumberIntVal(int64(nodeType.CPUCores)),
				"ram_megabytes":  cty.NumberIntVal(int64(nodeType.RAMMegabytes)),
				"disk_gigabytes": cty.NumberIntVal(int64(nodeType.DiskGigabytes)),
			})
		}
		cloudNodeTypesBody.SetAttributeValue(k, cty.ListVal(nodeTypeValues))
	}

	// Write the updated clouds file
	err := os.WriteFile(cloudsPath, f.Bytes(), 0644)
	if err != nil {
		return err
	}

	return nil
}

func loadCloudsFile() (CloudsFile, error) {
	cloudsPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "clouds.hcl")
	var cloudsFile CloudsFile

	data, err := os.ReadFile(cloudsPath)
	if err == nil {
		// Parse HCL file
		file, diags := hclsyntax.ParseConfig(data, cloudsPath, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return cloudsFile, fmt.Errorf("error parsing clouds.hcl: %s", diags)
		}

		// Extract data from HCL
		content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "last_updated"},
			},
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "cloud_regions"},
				{Type: "cloud_node_types"},
			},
		})
		if diags.HasErrors() {
			return cloudsFile, fmt.Errorf("error extracting content from clouds.hcl: %s", diags)
		}

		if attr, exists := content.Attributes["last_updated"]; exists {
			value, diags := attr.Expr.Value(nil)
			if !diags.HasErrors() {
				cloudsFile.LastUpdated = value.AsString()
			}
		}

		cloudsFile.CloudRegions = make(map[string][]string)
		cloudsFile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)

		for _, block := range content.Blocks {
			switch block.Type {
			case "cloud_regions":
				content, _, diags := block.Body.PartialContent(&hcl.BodySchema{
					Attributes: []hcl.AttributeSchema{
						{Name: "*"},
					},
				})
				if !diags.HasErrors() {
					for name, attr := range content.Attributes {
						values, diags := attr.Expr.Value(nil)
						if !diags.HasErrors() && values.CanIterateElements() {
							var regions []string
							it := values.ElementIterator()
							for it.Next() {
								_, value := it.Element()
								regions = append(regions, value.AsString())
							}
							cloudsFile.CloudRegions[name] = regions
						}
					}
				}
			case "cloud_node_types":
				content, _, diags := block.Body.PartialContent(&hcl.BodySchema{
					Attributes: []hcl.AttributeSchema{
						{Name: "*"},
					},
				})
				if !diags.HasErrors() {
					for name, attr := range content.Attributes {
						values, diags := attr.Expr.Value(nil)
						if !diags.HasErrors() && values.CanIterateElements() {
							var nodeTypes []InstanceSizeInfo
							it := values.ElementIterator()
							for it.Next() {
								_, value := it.Element()
								if value.Type().IsObjectType() {
									var nodeType InstanceSizeInfo
									nodeType.Name = value.GetAttr("name").AsString()
									cpuCores, _ := value.GetAttr("cpu_cores").AsBigFloat().Int64()
									nodeType.CPUCores = int(cpuCores)
									ramMB, _ := value.GetAttr("ram_megabytes").AsBigFloat().Int64()
									nodeType.RAMMegabytes = int(ramMB)
									diskGB, _ := value.GetAttr("disk_gigabytes").AsBigFloat().Int64()
									nodeType.DiskGigabytes = int(diskGB)
									nodeTypes = append(nodeTypes, nodeType)
								}
							}
							cloudsFile.CloudNodeTypes[name] = nodeTypes
						}
					}
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return cloudsFile, err
	}

	if cloudsFile.CloudRegions == nil {
		cloudsFile.CloudRegions = make(map[string][]string)
	}
	if cloudsFile.CloudNodeTypes == nil {
		cloudsFile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)
	}

	return cloudsFile, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func verifyIndexFile() {
	indexPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		log.Error("Failed to read index.hcl", "error", err)
		return
	}
	fmt.Println("Contents of index.hcl:")
	fmt.Println(string(content))
}

func simpleHCLParser(content string) map[string][]string {
	configs := make(map[string][]string)
	lines := strings.Split(content, "\n")
	inConfigsBlock := false
	currentConfig := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "configs {" {
			inConfigsBlock = true
			continue
		}
		if inConfigsBlock {
			if strings.HasSuffix(trimmedLine, "{") {
				currentConfig = strings.TrimSuffix(trimmedLine, " {")
				configs[currentConfig] = []string{}
			} else if trimmedLine == "}" {
				if currentConfig != "" {
					currentConfig = ""
				} else {
					inConfigsBlock = false
				}
			} else if strings.HasPrefix(trimmedLine, "files = [") {
				files := strings.Trim(strings.TrimPrefix(trimmedLine, "files = ["), "]")
				configs[currentConfig] = append(configs[currentConfig], strings.Split(files, ", ")...)
			}
		}
	}
	return configs
}

func cleanupIndexFile(indexFile *IndexFile) {
	for configName, config := range indexFile.Configs {
		cleanedFiles := make([]string, len(config.Files))
		for i, file := range config.Files {
			// Remove extra quotes and backslashes
			cleaned := strings.Trim(file, "\"\\")
			// Ensure forward slashes
			cleaned = filepath.ToSlash(cleaned)
			cleanedFiles[i] = cleaned
		}
		indexFile.Configs[configName] = Config{Files: cleanedFiles}
	}
}

func initializeAndCleanup() error {
	indexFile, err := loadIndexFile()
	if err != nil {
		return err
	}
	cleanupIndexFile(&indexFile)
	return updateIndexFile(CloudConfig{}, indexFile)
}

func getVersion() string {
	// Try to get the GitHub release version
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// If not available, try to get the last commit hash
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// If neither is available, return "unknown"
	return "unknown"
}

func printIntro() {
	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 3)

	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	// Create the title and version strings
	title := titleStyle.Render("k1space")
	version := versionStyle.Render("v" + getVersion())

	// Combine and print
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", version))
	fmt.Println()
}

// GitHubRelease represents the structure of a GitHub release
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
}

// getLatestGitHubRelease fetches the latest release information from GitHub
func getLatestGitHubRelease(owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}

// extractCommitHash attempts to extract the commit hash from the release body
func extractCommitHash(releaseBody string) string {
	lines := strings.Split(releaseBody, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Commit:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Commit:"))
		}
	}
	return ""
}

func fetchLatestReleaseInfo(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}

func downloadBinary(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp("", "k1space-*")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	_, err = tempFile.ReadFrom(resp.Body)
	if err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func printVersionInfo(logger *log.Logger) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	// Local install information
	localVersion := getVersion()

	fmt.Println(titleStyle.Render("\nLocal Install:"))
	fmt.Printf("Version: %s\n", infoStyle.Render(localVersion))

	// Remote/latest version information
	remoteRelease, err := getLatestGitHubRelease("ssotops", "k1space")
	if err != nil {
		logger.Error("Error fetching remote version info", "error", err)
		return
	}

	fmt.Println(titleStyle.Render("\nRemote/Latest Version:"))
	fmt.Printf("Version: %s\n", infoStyle.Render(remoteRelease.TagName))
	fmt.Printf("Released: %s\n", infoStyle.Render(remoteRelease.PublishedAt.Format(time.RFC3339)))

	// Extract commit hash from release body if available
	commitHash := extractCommitHash(remoteRelease.Body)
	if commitHash != "" {
		fmt.Printf("Commit Hash: %s\n", infoStyle.Render(commitHash))
	} else {
		fmt.Println("Commit Hash: Not available")
	}
}

func printConfigPaths(logger *log.Logger) {
	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space")

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	fmt.Println(titleStyle.Render("\n📂 K1space Base Directory:"))
	fmt.Printf("   %s\n\n", pathStyle.Render(fmt.Sprintf("cd %s", baseDir)))

	fmt.Println(titleStyle.Render("📄 K1space Config Files:"))
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".hcl" || filepath.Base(path) == ".local.cloud.env") {
			fmt.Printf("   %s\n", pathStyle.Render(path))
		}
		return nil
	})
	if err != nil {
		logger.Error("Error walking through base directory", "error", err)
	}

	fmt.Println(titleStyle.Render("\n📁 K1space Cloud Directories:"))
	cloudDirs, err := os.ReadDir(baseDir)
	if err != nil {
		logger.Error("Error reading base directory", "error", err)
		return
	}
	for _, dir := range cloudDirs {
		if dir.IsDir() && dir.Name() != ".cache" && dir.Name() != ".repositories" {
			fmt.Printf("   %s\n", pathStyle.Render(filepath.Join(baseDir, dir.Name())))
		}
	}

	fmt.Println() // Add an extra newline for spacing
}

func upgradeK1space(logger *log.Logger) {
	logger.Info("Upgrading k1space...")

	// Define repository details
	repo := "ssotops/k1space"
	binary := "k1space"

	// Determine OS and architecture
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Fetch the latest release information
	logger.Info("Fetching latest release information...")
	releaseInfo, err := fetchLatestReleaseInfo(repo)
	if err != nil {
		logger.Error("Failed to fetch latest release information", "error", err)
		return
	}

	version := releaseInfo.TagName
	logger.Info("Latest version", "version", version)

	// Construct the download URL for the specific asset
	assetName := fmt.Sprintf("%s_%s_%s", binary, osName, arch)
	if osName == "windows" {
		assetName += ".exe"
	}
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, version, assetName)

	// Download the binary
	logger.Info("Downloading new version", "version", version, "os", osName, "arch", arch)
	tempFile, err := downloadBinary(downloadURL)
	if err != nil {
		logger.Error("Failed to download binary", "error", err)
		return
	}
	defer os.Remove(tempFile)

	// Make it executable (skip for Windows)
	if osName != "windows" {
		err = os.Chmod(tempFile, 0755)
		if err != nil {
			logger.Error("Failed to make binary executable", "error", err)
			return
		}
	}

	// Get the path of the current executable
	execPath, err := os.Executable()
	if err != nil {
		logger.Error("Failed to get current executable path", "error", err)
		return
	}

	// Replace the current binary with the new one
	err = os.Rename(tempFile, execPath)
	if err != nil {
		logger.Error("Failed to replace current binary", "error", err)
		return
	}

	logger.Info("k1space has been successfully upgraded!", "version", version)
}

func promptKubefirstBinary() (string, error) {
	var useGlobal bool
	err := huh.NewConfirm().
		Title("Do you want to use the global kubefirst installation?").
		Value(&useGlobal).
		Run()

	if err != nil {
		return "", err
	}

	if useGlobal {
		path, err := exec.LookPath("kubefirst")
		if err != nil {
			return "", fmt.Errorf("global kubefirst not found: %w", err)
		}
		version, err := exec.Command(path, "version").Output()
		if err != nil {
			return "", fmt.Errorf("error getting kubefirst version: %w", err)
		}
		fmt.Printf("Using global kubefirst: %s\nVersion: %s\n", path, version)
		return path, nil
	}

	var localPath string
	err = huh.NewInput().
		Title("Enter the path to the local kubefirst binary").
		Value(&localPath).
		Run()

	if err != nil {
		return "", err
	}

	return localPath, nil
}

func fetchKubefirstFlags(kubefirstPath, cloudProvider string) (map[string]string, error) {
	cmd := exec.Command(kubefirstPath, strings.ToLower(cloudProvider), "create", "--help")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error running kubefirst help: %w", err)
	}

	flags := make(map[string]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "--") {
			parts := strings.SplitN(trimmedLine, " ", 2)
			if len(parts) > 0 {
				flag := strings.TrimPrefix(parts[0], "--")
				flag = strings.TrimSuffix(flag, ",")
				description := ""
				if len(parts) > 1 {
					description = strings.TrimSpace(parts[1])
				}
				flags[flag] = description
			}
		}
	}

	return flags, nil
}


