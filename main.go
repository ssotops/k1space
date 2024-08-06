package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/civo/civogo"
	"github.com/digitalocean/godo"
	"github.com/fatih/color"
)

type CloudConfig struct {
	StaticPrefix string
	CloudPrefix  string
	Region       string
	Flags        map[string]string
}

type InstanceSizeInfo struct {
	Name          string
	CPUCores      int
	RAMMegabytes  int
	DiskGigabytes int
}

type Lockfile struct {
	Version        int                           `json:"version"`
	Configs        map[string][]string           `json:"configs"`
	DefaultValues  map[string]string             `json:"defaultValues"`
	CloudRegions   map[string][]string           `json:"cloudRegions"`
	CloudNodeTypes map[string][]InstanceSizeInfo `json:"cloudNodeTypes"`
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
	config := CloudConfig{
		Flags: make(map[string]string),
	}

	lockfile, err := loadLockfile()
	if err != nil {
		fmt.Println("Error loading lockfile:", err)
		return
	}

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
		fmt.Println("Error:", err)
		return
	}

	if config.CloudPrefix == "DigitalOcean" {
		err = updateDigitalOceanRegions(&lockfile)
		if err != nil {
			fmt.Printf("Error updating %s regions: %v\n", config.CloudPrefix, err)
			return
		}
		err = updateDigitalOceanNodeTypes(&lockfile)
		if err != nil {
			fmt.Printf("Error updating %s node types: %v\n", config.CloudPrefix, err)
			return
		}
	} else if config.CloudPrefix == "Civo" {
		err = updateCivoRegions(&lockfile)
		if err != nil {
			fmt.Printf("Error updating %s regions: %v\n", config.CloudPrefix, err)
			return
		}
		err = updateCivoNodeTypes(&lockfile)
		if err != nil {
			fmt.Printf("Error updating %s node types: %v\n", config.CloudPrefix, err)
			return
		}
	}

	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Options(getRegionOptions(config.CloudPrefix, lockfile)...).
				Value(&config.Region),
		),
	)

	err = regionForm.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	flags := cloudFlags[config.CloudPrefix]
	if len(flags) == 0 {
		fmt.Println("No flags found for the selected cloud provider.")
		return
	}

	flagInputs := make([]struct{ Name, Value string }, 0, len(flags))
	flagGroups := make([]huh.Field, 0, len(flags))
	for _, flag := range flags {
		if flag == "cloud-region" || flag == "node-type" {
			continue // Skip cloud-region and node-type as we've already set them
		}

		defaultValue := lockfile.DefaultValues[flag]
		flagInput := struct{ Name, Value string }{Name: flag, Value: defaultValue}
		flagInputs = append(flagInputs, flagInput)
		flagGroups = append(flagGroups,
			huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Placeholder(defaultValue).
				Value(&flagInputs[len(flagInputs)-1].Value),
		)
	}

	var selectedNodeType string
	nodeTypeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select node type").
				Options(getNodeTypeOptions(config.CloudPrefix, lockfile)...).
				Value(&selectedNodeType),
		),
	)

	err = nodeTypeForm.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Extract the actual node type name from the selected value
	nodeTypeName := strings.Split(selectedNodeType, " ")[0]

	// Set the node-type flag to the selected node type name
	config.Flags["node-type"] = nodeTypeName

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)

	err = flagForm.Run()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Update config.Flags and lockfile.DefaultValues with the collected values
	for _, fi := range flagInputs {
		config.Flags[fi.Name] = fi.Value
		lockfile.DefaultValues[fi.Name] = fi.Value
	}

	// Ensure cloud-region and node-type is set in the lockfile
	lockfile.DefaultValues["cloud-region"] = config.Region
	lockfile.DefaultValues["node-type"] = selectedNodeType

	err = generateFiles(config)
	if err != nil {
		fmt.Println("Error generating files:", err)
		return
	}

	err = updateLockfile(config, lockfile)
	if err != nil {
		fmt.Println("Error updating lockfile:", err)
		return
	}
	// Define baseDir
	baseDir := filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))

	// Pretty-print the summary
	color.New(color.FgGreen, color.Bold).Println("\n✅ Configuration completed successfully! Summary:")
	fmt.Println()

	// Create color functions
	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("%s %s\n", bold("☁️  Cloud Provider:"), cyan(config.CloudPrefix))
	fmt.Printf("%s %s\n", bold("🌎 Region:"), cyan(config.Region))
	fmt.Printf("%s %s\n", bold("💻 Node Type:"), cyan(selectedNodeType))

	// Print relevant file paths
	color.New(color.FgYellow, color.Bold).Println("\n📁 Generated Files:")
	filePrefix := "  "
	fmt.Printf("%s%s %s\n", filePrefix, bold("Init Script:"), filepath.Join(baseDir, "00-init.sh"))
	fmt.Printf("%s%s %s\n", filePrefix, bold("Kubefirst Script:"), filepath.Join(baseDir, "01-kubefirst-cloud.sh"))
	fmt.Printf("%s%s %s\n", filePrefix, bold("Environment File:"), filepath.Join(baseDir, ".local.cloud.env"))

	// Print command to run the generated init script
	color.New(color.FgMagenta, color.Bold).Println("\n🚀 To run the initialization script, use the following command:")
	color.New(color.FgHiBlue).Printf("cd %s && ./00-init.sh\n", baseDir)
}

func getCivoClient() (*civogo.Client, error) {
	token := os.Getenv("CIVO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("CIVO_TOKEN not found in environment. Please set it and try again")
	}
	return civogo.NewClient(token, "")
}

func updateCivoRegions(lockfile *Lockfile) error {
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

	lockfile.CloudRegions["Civo"] = regionCodes
	return nil
}

func updateCivoNodeTypes(lockfile *Lockfile) error {
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

	lockfile.CloudNodeTypes["Civo"] = sizeInfos
	return nil
}

func getCloudProviderOptions() []huh.Option[string] {
	options := make([]huh.Option[string], len(cloudProviders))
	for i, provider := range cloudProviders {
		options[i] = huh.Option[string]{Key: provider, Value: provider}
	}
	return options
}

func getRegionOptions(cloudProvider string, lockfile Lockfile) []huh.Option[string] {
	regions := lockfile.CloudRegions[cloudProvider]
	options := make([]huh.Option[string], len(regions))
	for i, region := range regions {
		options[i] = huh.Option[string]{Key: region, Value: region}
	}
	return options
}

func getNodeTypeOptions(cloudProvider string, lockfile Lockfile) []huh.Option[string] {
	nodeTypes := lockfile.CloudNodeTypes[cloudProvider]
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

func loadLockfile() (Lockfile, error) {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")
	var lockfile Lockfile

	data, err := os.ReadFile(lockfilePath)
	if err == nil {
		var tempMap map[string]interface{}
		err = json.Unmarshal(data, &tempMap)
		if err != nil {
			return lockfile, err
		}

		version, ok := tempMap["version"].(float64)
		if !ok || version < 2 {
			// Old version or no version, migrate data
			err = json.Unmarshal(data, &struct {
				Configs        map[string][]string `json:"configs"`
				DefaultValues  map[string]string   `json:"defaultValues"`
				CloudRegions   map[string][]string `json:"cloudRegions"`
				CloudNodeTypes map[string][]string `json:"cloudNodeTypes"`
			}{
				Configs:        lockfile.Configs,
				DefaultValues:  lockfile.DefaultValues,
				CloudRegions:   lockfile.CloudRegions,
				CloudNodeTypes: make(map[string][]string),
			})
			if err != nil {
				return lockfile, err
			}
			// Convert old CloudNodeTypes to new format
			lockfile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)
		} else {
			// Current version, unmarshal directly
			err = json.Unmarshal(data, &lockfile)
			if err != nil {
				return lockfile, err
			}
		}
	} else if !os.IsNotExist(err) {
		return lockfile, err
	}

	// Initialize maps if they don't exist
	if lockfile.Configs == nil {
		lockfile.Configs = make(map[string][]string)
	}
	if lockfile.DefaultValues == nil {
		lockfile.DefaultValues = make(map[string]string)
	}
	if lockfile.CloudRegions == nil {
		lockfile.CloudRegions = make(map[string][]string)
	}
	if lockfile.CloudNodeTypes == nil {
		lockfile.CloudNodeTypes = make(map[string][]InstanceSizeInfo)
	}

	// Set the current version
	lockfile.Version = 2

	return lockfile, nil
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

func updateDigitalOceanNodeTypes(lockfile *Lockfile) error {
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

	lockfile.CloudNodeTypes["DigitalOcean"] = sizeInfos
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

func updateDigitalOceanRegions(lockfile *Lockfile) error {
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

	lockfile.CloudRegions["DigitalOcean"] = regionSlugs
	return nil
}

func generateFiles(config CloudConfig) error {
	baseDir := filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
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

func updateLockfile(config CloudConfig, lockfile Lockfile) error {
	lockfilePath := filepath.Join(os.Getenv("HOME"), ".k1space", "k1.locked.json")

	// Add the new configuration
	key := fmt.Sprintf("%s_%s", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region))
	lockfile.Configs[key] = []string{
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "00-init.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), "01-kubefirst-cloud.sh"),
		filepath.Join(os.Getenv("HOME"), ".k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), ".local.cloud.env"),
	}

	// Ensure cloud-region and node-type are set in the lockfile
	lockfile.DefaultValues["cloud-region"] = config.Region
	lockfile.DefaultValues["node-type"] = config.Flags["node-type"]

	// Set the current version
	lockfile.Version = 2

	// Write the updated lockfile
	data, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(lockfilePath, data, 0644)
}
