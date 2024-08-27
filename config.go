package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func createConfig(config *CloudConfig) {
	if config == nil {
		log.Error("config is nil")
		return
	}

	log.Info("Starting createConfig function")

	log.Info("CloudConfig initialized", "config", fmt.Sprintf("%+v", config))

	defer func() {
		log.Info("Final config state", "config", fmt.Sprintf("%+v", config))
	}()

	if config.Flags == nil {
		config.Flags = &sync.Map{}
		log.Info("Reinitializing config.Flags")
	}

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
	log.Info("Config state after fetching kubefirst flags", "config", fmt.Sprintf("%+v", config))

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
				Value(&flagInputs[len(flagInputs)-1].Value)
		case "node-type":
			field = huh.NewSelect[string]().
				Title("Select node type").
				Description(description).
				Options(getNodeTypeOptions(config.CloudPrefix, cloudsFile)...).
				Value(&flagInputs[len(flagInputs)-1].Value)
		default:
			field = huh.NewInput().
				Title(fmt.Sprintf("Enter value for %s", flag)).
				Description(description).
				Placeholder(defaultValue).
				Value(&flagInputs[len(flagInputs)-1].Value)
		}

		flagGroups = append(flagGroups, field)
	}

	flagForm := huh.NewForm(
		huh.NewGroup(flagGroups...),
	)
	log.Info("Config state before flag input form", "config", fmt.Sprintf("%+v", config))

	err = flagForm.Run()
	if err != nil {
		log.Error("Error in flag input form", "error", err)
		return
	}

	log.Info("Debug: Right before updating config.Flags in loop", "config", fmt.Sprintf("%+v", config))
	for i, fi := range flagInputs {
		log.Info("Debug: Starting flag update", "index", i, "name", fi.Name, "value", fi.Value)
		config.Flags.Store(fi.Name, fi.Value)
		log.Info("Debug: After updating flag", "index", i, "config", fmt.Sprintf("%+v", config))

		if fi.Name == "node-type" {
			config.SelectedNodeType = fi.Value
		}
		if fi.Name == "cloud-region" {
			config.Region = fi.Value
		}
	}
	log.Info("Debug: After flag update loop", "config", fmt.Sprintf("%+v", config))

	log.Info("After updating flags", "config", fmt.Sprintf("%+v", config))

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

	cleanupIndexFile(&indexFile)

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
			// Remove any existing quotes and escape characters
			cleanedFile := strings.Trim(file, "\"\\")
			// Convert to forward slashes for consistency
			cleanedFile = filepath.ToSlash(cleanedFile)
			fileValues[i] = cty.StringVal(cleanedFile)
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

func updateIndexFile(config *CloudConfig, indexFile IndexFile) error {
	log.Info("Starting updateIndexFile function", "config", fmt.Sprintf("%+v", config), "indexFile", fmt.Sprintf("%+v", indexFile))

	indexPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "index.hcl")

	// Update LastUpdated
	indexFile.LastUpdated = time.Now().UTC().Format(time.RFC3339)

	// Add or update the new configuration
	if config.CloudPrefix != "" && config.Region != "" && config.StaticPrefix != "" {
		key := fmt.Sprintf("%s_%s_%s", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix)

		newConfig := Config{
			Files: []string{
				filepath.ToSlash(filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix, "00-init.sh")),
				filepath.ToSlash(filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix, "01-kubefirst-cloud.sh")),
				filepath.ToSlash(filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix, ".local.cloud.env")),
			},
			Flags: make(map[string]string),
		}

		if existingConfig, exists := indexFile.Configs[key]; exists {
			// Clean up existing file paths
			for i, file := range existingConfig.Files {
				newConfig.Files[i] = filepath.ToSlash(strings.Trim(file, "\"\\"))
			}
			// Copy existing flags
			for k, v := range existingConfig.Flags {
				newConfig.Flags[k] = v
			}
		}

		// Update the Flags if config.Flags is not nil
		if config.Flags != nil {
			config.Flags.Range(func(k, v interface{}) bool {
				newConfig.Flags[k.(string)] = v.(string)
				return true
			})
		}

		// Assign the new or updated config back to the map
		indexFile.Configs[key] = newConfig

		// Update default values
		if indexFile.DefaultValues == nil {
			indexFile.DefaultValues = make(map[string]string)
		}
		if config.Flags != nil {
			config.Flags.Range(func(k, v interface{}) bool {
				indexFile.DefaultValues[k.(string)] = v.(string)
				return true
			})
		}
	}

	// Clean up all file paths in the index file
	for configKey, config := range indexFile.Configs {
		cleanedConfig := Config{
			Files: make([]string, len(config.Files)),
			Flags: make(map[string]string),
		}
		for i, file := range config.Files {
			cleanedConfig.Files[i] = filepath.ToSlash(strings.Trim(file, "\"\\"))
		}
		for k, v := range config.Flags {
			cleanedConfig.Flags[k] = v
		}
		indexFile.Configs[configKey] = cleanedConfig
	}

	log.Info("Updated indexFile", "indexFile", fmt.Sprintf("%+v", indexFile))

	return createOrUpdateIndexFile(indexPath, indexFile)
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

func updateCloudsFile(config *CloudConfig, cloudsFile CloudsFile) error {
	cloudsPath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", "clouds.hcl")

	// Update cloud regions
	if _, exists := cloudsFile.CloudRegions[config.CloudPrefix]; !exists {
		cloudsFile.CloudRegions[config.CloudPrefix] = []string{}
	}
	if !contains(cloudsFile.CloudRegions[config.CloudPrefix], config.Region) {
		cloudsFile.CloudRegions[config.CloudPrefix] = append(
			cloudsFile.CloudRegions[config.CloudPrefix],
			config.Region,
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

func generateFiles(config *CloudConfig, kubefirstPath string) error {
	log.Info("Starting generateFiles function", "config", fmt.Sprintf("%+v", config))

	baseDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix)
	err := os.MkdirAll(baseDir, 0755)
	if err != nil {
		log.Error("Error creating directory", "error", err)
		return err
	}

	// Generate .local.cloud.env
	envContent := generateEnvContent(config)
	log.Info("Generated env content", "content", envContent)
	envFilePath := filepath.Join(baseDir, ".local.cloud.env")
	err = os.WriteFile(envFilePath, []byte(envContent), 0644)
	if err != nil {
		log.Error("Error writing .local.cloud.env", "error", err)
		return err
	}
	log.Info("Generated .local.cloud.env", "path", envFilePath)

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

func generateEnvContent(config *CloudConfig) string {
	var content strings.Builder
	prefix := fmt.Sprintf("%s_%s_%s", config.StaticPrefix, strings.ToUpper(config.CloudPrefix), strings.ToUpper(config.Region))
	config.Flags.Range(func(k, v interface{}) bool {
		flag := k.(string)
		value := v.(string)
		envVarName := fmt.Sprintf("%s_%s", prefix, strings.ToUpper(strings.ReplaceAll(flag, "-", "_")))
		content.WriteString(fmt.Sprintf("export %s=\"%s\"\n", envVarName, value))
		return true
	})
	return content.String()
}

func generateInitContent() string {
	return `#!/bin/bash
op run --env-file="./.local.cloud.env" -- sh ./01-kubefirst-cloud.sh
`
}

func generateKubefirstContent(config *CloudConfig, kubefirstPath string) string {
	var content strings.Builder
	content.WriteString("#!/bin/bash\n\n")
	content.WriteString("./prepare/01-check-dependencies.sh\n\n")
	content.WriteString(fmt.Sprintf("%s %s create \\\n", kubefirstPath, strings.ToLower(config.CloudPrefix)))

	prefix := fmt.Sprintf("%s_%s_%s", config.StaticPrefix, strings.ToUpper(config.CloudPrefix), strings.ToUpper(config.Region))
	flags := make([]string, 0)
	config.Flags.Range(func(k, v interface{}) bool {
		flag := k.(string)
		envVarName := fmt.Sprintf("%s_%s", prefix, strings.ToUpper(strings.ReplaceAll(flag, "-", "_")))
		flags = append(flags, fmt.Sprintf("  --%s \"$%s\"", flag, envVarName))
		return true
	})

	content.WriteString(strings.Join(flags, " \\\n"))
	content.WriteString("\n")

	return content.String()
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

func convertStringSliceToCtyValueSlice(slice []string) []cty.Value {
	values := make([]cty.Value, len(slice))
	for i, s := range slice {
		values[i] = cty.StringVal(s)
	}
	return values
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func promptKubefirstBinary() (string, error) {
	var useLocal bool
	err := huh.NewConfirm().
		Title("Do you want to use a local kubefirst installation? (No for global)").
		Value(&useLocal).
		Run()

	if err != nil {
		return "", err
	}

	if !useLocal {
		path, err := exec.LookPath("kubefirst")
		if err != nil {
			return "", fmt.Errorf("global kubefirst not found: %w", err)
		}
		version, err := exec.Command(path, "version").Output()
		if err != nil {
			return "", fmt.Errorf("error getting kubefirst version: %w", err)
		}
		fmt.Printf("Using global kubefirst: %s\nVersion: %s\n", path, string(version))
		return path, nil
	}

	// Local option sub-menu
	var localOption string
	err = huh.NewSelect[string]().
		Title("Choose the local kubefirst option:").
		Options(
			huh.NewOption("Use ~/.ssot/k1space/.repositories/kubefirst/kubefirst", "repo"),
			huh.NewOption("Specify a custom path", "custom"),
		).
		Value(&localOption).
		Run()

	if err != nil {
		return "", err
	}

	switch localOption {
	case "repo":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("error getting user home directory: %w", err)
		}
		repoPath := filepath.Join(homeDir, ".ssot", "k1space", ".repositories", "kubefirst", "kubefirst")

		// Check if the file exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			return "", fmt.Errorf("kubefirst binary not found at %s", repoPath)
		}

		log.Info("Using kubefirst binary from repository", "path", repoPath)

		// Return the absolute path
		return repoPath, nil
	case "custom":
		var customPath string
		err = huh.NewInput().
			Title("Enter the path to the local kubefirst binary").
			Value(&customPath).
			Run()

		if err != nil {
			return "", err
		}

		// Check if the file exists
		if _, err := os.Stat(customPath); os.IsNotExist(err) {
			return "", fmt.Errorf("kubefirst binary not found at %s", customPath)
		}

		log.Info("Using custom kubefirst binary", "path", customPath)
		return customPath, nil
	default:
		return "", fmt.Errorf("invalid local option selected")
	}
}

func fetchKubefirstFlags(kubefirstPath, cloudProvider string) (map[string]string, error) {
	cmd := exec.Command(kubefirstPath, strings.ToLower(cloudProvider), "create", "--help")
	log.Info("Executing kubefirst command", "path", kubefirstPath, "args", cmd.Args)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error running kubefirst help: %w\nOutput: %s", err, string(output))
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
		indexFile.Configs[configName] = Config{Files: cleanedFiles, Flags: config.Flags}
	}
}

func deleteConfig() {
	log.Info("Starting deleteConfig function")

	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the index.hcl file exists and is correctly formatted.")
		return
	}

	if len(indexFile.Configs) == 0 {
		fmt.Println("No configurations found to delete.")
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
				Title("Select a configuration to delete").
				Options(configOptions...).
				Value(&selectedConfig),
		),
	)

	err = form.Run()
	if err != nil {
		log.Error("Error in config selection", "error", err)
		return
	}

	var confirmDelete bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Are you sure you want to delete the configuration '%s'?", selectedConfig)).
				Value(&confirmDelete),
		),
	)

	err = confirmForm.Run()
	if err != nil {
		log.Error("Error in delete confirmation", "error", err)
		return
	}

	if !confirmDelete {
		fmt.Println("Deletion cancelled.")
		return
	}

	// Extract cloud, region, and prefix from the selected config
	parts := strings.Split(selectedConfig, "_")
	if len(parts) != 3 {
		log.Error("Invalid config name format", "config", selectedConfig)
		fmt.Println("Invalid configuration name format. Deletion cancelled.")
		return
	}
	cloud, region, prefix := parts[0], parts[1], parts[2]

	// Create .cache directory if it doesn't exist
	cacheDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", ".cache")
	err = os.MkdirAll(cacheDir, 0755)
	if err != nil {
		log.Error("Error creating .cache directory", "error", err)
		fmt.Println("Failed to create .cache directory. Deletion cancelled.")
		return
	}

	// Backup the config directory
	sourceDir := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", cloud, region, prefix)
	backupDir := filepath.Join(cacheDir, fmt.Sprintf("%s_%s", selectedConfig, time.Now().Format("20060102_150405")))

	err = os.Rename(sourceDir, backupDir)
	if err != nil {
		log.Error("Error backing up config directory", "error", err)
		fmt.Println("Failed to backup configuration directory. Deletion cancelled.")
		return
	}

	// Delete the config from index.hcl
	delete(indexFile.Configs, selectedConfig)
	err = updateIndexFile(&CloudConfig{Flags: &sync.Map{}}, indexFile)
	if err != nil {
		log.Error("Error updating index file", "error", err)
		fmt.Printf("Failed to update index file. The configuration '%s' has been backed up but not removed from the index.\n", selectedConfig)
		// Attempt to restore the backed up directory
		os.Rename(backupDir, sourceDir)
		return
	}

	fmt.Printf("Configuration '%s' has been deleted and backed up to %s\n", selectedConfig, backupDir)
	log.Info("deleteConfig function completed successfully")

	// Final confirmation
	var finalConfirm bool
	finalConfirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("The configuration has been deleted. Press Enter to continue.").
				Value(&finalConfirm),
		),
	)
	finalConfirmForm.Run()
}

func listConfigs() {
	log.Info("Starting listConfigs function")

	indexFile, err := loadIndexFile()
	if err != nil {
		log.Error("Error loading index file", "error", err)
		fmt.Println("Failed to load configurations. Please ensure that the index.hcl file exists and is correctly formatted.")
		return
	}

	if len(indexFile.Configs) == 0 {
		fmt.Println("No configurations found.")
		return
	}

	fmt.Println(style.Render("Existing Configurations:"))
	for configName, config := range indexFile.Configs {
		parts := strings.Split(configName, "_")
		if len(parts) == 3 {
			cloud, region, prefix := parts[0], parts[1], parts[2]
			fmt.Printf("\n%s:\n", style.Render(configName))
			fmt.Printf("  Cloud Provider: %s\n", cloud)
			fmt.Printf("  Region: %s\n", region)
			fmt.Printf("  Prefix: %s\n", prefix)
			fmt.Printf("  Files:\n")
			for _, file := range config.Files {
				fmt.Printf("    - %s\n", file)
			}
		} else {
			fmt.Printf("\n%s: (Invalid format)\n", style.Render(configName))
		}
	}

	// Wait for user input before returning to the menu
	fmt.Print("\nPress Enter to continue...")
	fmt.Scanln()
}
