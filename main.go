package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	StaticPrefix string
	CloudPrefix  string
	Region       string
	Flags        map[string]string
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

	for {
		action := runMainMenu()
		switch action {
		case "Config":
			runConfigMenu()
		case "Kubefirst":
			runKubefirstMenu()
		case "Provision Cluster":
			provisionCluster()
		case "Exit":
			fmt.Println("Exiting k1space. Goodbye!")
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
					huh.NewOption("Provision Cluster", "Provision Cluster"),
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
		fileOptions = append(fileOptions, huh.NewOption(filepath.Base(file), file))
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
	content, err := os.ReadFile(selectedFile)
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter static prefix").
				Description("Default is 'CS'").
				Placeholder("CS").
				Value(&config.StaticPrefix),

			huh.NewSelect[string]().
				Title("Select cloud provider").
				Options(getCloudProviderOptions()...).
				Value(&config.CloudPrefix),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in initial config form", "error", err)
		return
	}
	log.Info("Initial form completed", "StaticPrefix", config.StaticPrefix, "CloudPrefix", config.CloudPrefix)

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

	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Options(getRegionOptions(config.CloudPrefix, cloudsFile)...).
				Value(&config.Region),
		),
	)

	err = regionForm.Run()
	if err != nil {
		log.Error("Error in region selection", "error", err)
		return
	}
	log.Info("Region selected", "Region", config.Region)

	flags := cloudFlags[config.CloudPrefix]
	if len(flags) == 0 {
		log.Error("No flags found for the selected cloud provider")
		return
	}
	log.Info("Flags retrieved for cloud provider", "Flags", flags)

	flagInputs := make([]struct{ Name, Value string }, 0, len(flags))
	flagGroups := make([]huh.Field, 0, len(flags))
	for _, flag := range flags {
		if flag == "cloud-region" || flag == "node-type" {
			continue // Skip cloud-region and node-type as we've already set them
		}

		defaultValue := indexFile.DefaultValues[flag]
		flagInput := struct{ Name, Value string }{Name: flag, Value: defaultValue}
		flagInputs = append(flagInputs, flagInput)
		flagGroups = append(flagGroups,
			huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Placeholder(defaultValue).
				Value(&flagInputs[len(flagInputs)-1].Value),
		)
	}
	log.Info("Flag inputs and groups prepared", "FlagInputs", flagInputs)

	var selectedNodeType string
	nodeTypeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select node type").
				Options(getNodeTypeOptions(config.CloudPrefix, cloudsFile)...).
				Value(&selectedNodeType),
		),
	)

	err = nodeTypeForm.Run()
	if err != nil {
		log.Error("Error in node type selection", "error", err)
		return
	}
	log.Info("Node type selected", "SelectedNodeType", selectedNodeType)

	// Extract the actual node type name from the selected value
	nodeTypeName := strings.Split(selectedNodeType, " ")[0]

	// Set the node-type flag to the selected node type name
	if config.Flags == nil {
		log.Error("config.Flags is nil, reinitializing")
		config.Flags = make(map[string]string)
	}
	config.Flags["node-type"] = nodeTypeName
	log.Info("Node type set in config.Flags", "NodeType", nodeTypeName)

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		log.Error("Error in flag input form", "error", err)
		return
	}
	log.Info("Flag input form completed")

	// Update config.Flags and indexFile.DefaultValues with the collected values
	for _, fi := range flagInputs {
		if config.Flags == nil {
			log.Error("config.Flags is nil, reinitializing")
			config.Flags = make(map[string]string)
		}
		config.Flags[fi.Name] = fi.Value
		if indexFile.DefaultValues == nil {
			log.Error("indexFile.DefaultValues is nil, initializing")
			indexFile.DefaultValues = make(map[string]string)
		}
		indexFile.DefaultValues[fi.Name] = fi.Value
	}
	log.Info("Updated config.Flags and indexFile.DefaultValues", "ConfigFlags", config.Flags, "IndexFileDefaultValues", indexFile.DefaultValues)

	// Ensure cloud-region and node-type is set in the indexFile
	if indexFile.DefaultValues == nil {
		log.Error("indexFile.DefaultValues is nil, initializing")
		indexFile.DefaultValues = make(map[string]string)
	}
	indexFile.DefaultValues["cloud-region"] = config.Region
	indexFile.DefaultValues["node-type"] = selectedNodeType
	log.Info("Set cloud-region and node-type in indexFile.DefaultValues")

	err = generateFiles(config)
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
	fmt.Printf("💻 Node Type: %s\n", selectedNodeType)

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

func updateIndexFile(config CloudConfig, indexFile IndexFile) error {
	indexPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")

	// Backup existing index file
	err := backupIndexFile(indexFile)
	if err != nil {
		return fmt.Errorf("error backing up index file: %w", err)
	}

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
		configBlock.Body().SetAttributeValue("files", cty.ListVal(convertStringSliceToCtyValueSlice(v.Files)))
	}

	// Write default_values
	defaultValuesBlock := rootBody.AppendNewBlock("default_values", nil)
	defaultValuesBody := defaultValuesBlock.Body()
	for k, v := range indexFile.DefaultValues {
		defaultValuesBody.SetAttributeValue(k, cty.StringVal(v))
	}

	// Write the updated index file
	err = os.WriteFile(indexPath, f.Bytes(), 0644)
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

func generateFiles(config CloudConfig) error {
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
	kubefirstContent := generateKubefirstContent(config)
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

func generateKubefirstContent(config CloudConfig) string {
	var content strings.Builder
	content.WriteString("#!/bin/bash\n\n")
	content.WriteString("./prepare/01-check-dependencies.sh\n\n")
	content.WriteString(fmt.Sprintf("kubefirst %s create \\\n", strings.ToLower(config.CloudPrefix)))

	for flag := range config.Flags {
		content.WriteString(fmt.Sprintf("  --%s $%s_%s_%s_%s \\\n", flag, config.StaticPrefix, config.CloudPrefix, strings.ToUpper(config.Region), strings.ToUpper(flag)))
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
