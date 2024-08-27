package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

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

	indexFile.Configs = configs
	for configName, config := range configs {
		log.Info("Parsed config", "name", configName, "fileCount", len(config.Files))
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
		if len(v.Files) > 0 {
			fileValues := make([]cty.Value, len(v.Files))
			for i, file := range v.Files {
				// Remove any existing quotes and escape characters
				cleanedFile := strings.Trim(file, "\"\\")
				// Convert to forward slashes for consistency
				cleanedFile = filepath.ToSlash(cleanedFile)
				fileValues[i] = cty.StringVal(cleanedFile)
			}
			configBlock.Body().SetAttributeValue("files", cty.ListVal(fileValues))
		} else {
			// If there are no files, set an empty list
			configBlock.Body().SetAttributeValue("files", cty.ListValEmpty(cty.String))
		}

		// Add flags as a new block
		if len(v.Flags) > 0 {
			flagsBlock := configBlock.Body().AppendNewBlock("flags", nil)
			for flagKey, flagValue := range v.Flags {
				flagsBlock.Body().SetAttributeValue(flagKey, cty.StringVal(flagValue))
			}
		}
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

		// Update the Flags
		config.Flags.Range(func(k, v interface{}) bool {
			if v.(string) != "" {
				newConfig.Flags[k.(string)] = v.(string)
			}
			return true
		})

		// Ensure all expected flags are present
		expectedFlags := []string{"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"}
		for _, flag := range expectedFlags {
			if _, exists := newConfig.Flags[flag]; !exists {
				newConfig.Flags[flag] = ""
			}
		}

		// Update or add the new configuration
		indexFile.Configs[key] = newConfig

		// Update default values
		if indexFile.DefaultValues == nil {
			indexFile.DefaultValues = make(map[string]string)
		}
		for k, v := range newConfig.Flags {
			if v != "" {
				indexFile.DefaultValues[k] = v
			}
		}
	}

	// Clean up configs
	for key, cfg := range indexFile.Configs {
		if cfg.Flags == nil {
			cfg.Flags = make(map[string]string)
		}
		// Ensure all expected flags are present for existing configs
		expectedFlags := []string{"alerts-email", "cloud-region", "cluster-name", "domain-name", "github-org", "dns-provider", "node-type"}
		for _, flag := range expectedFlags {
			if _, exists := cfg.Flags[flag]; !exists {
				cfg.Flags[flag] = ""
			}
		}
		indexFile.Configs[key] = cfg
	}

	log.Info("Updated indexFile", "indexFile", fmt.Sprintf("%+v", indexFile))

	return createOrUpdateIndexFile(indexPath, indexFile)
}

func simpleHCLParser(content string) map[string]Config {
	configs := make(map[string]Config)
	lines := strings.Split(content, "\n")
	inConfigsBlock := false
	currentConfig := ""
	inFlagsBlock := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "configs {" {
			inConfigsBlock = true
			continue
		}
		if inConfigsBlock {
			if strings.HasSuffix(trimmedLine, "{") {
				currentConfig = strings.TrimSuffix(trimmedLine, " {")
				configs[currentConfig] = Config{Files: []string{}, Flags: make(map[string]string)}
			} else if trimmedLine == "}" {
				if inFlagsBlock {
					inFlagsBlock = false
				} else if currentConfig != "" {
					currentConfig = ""
				} else {
					inConfigsBlock = false
				}
			} else if strings.HasPrefix(trimmedLine, "files = [") {
				files := strings.Trim(strings.TrimPrefix(trimmedLine, "files = ["), "]")
				if files != "" {
					filesList := strings.Split(files, ", ")
					for i := range filesList {
						filesList[i] = strings.Trim(filesList[i], "\"")
					}
					// Create a new Config struct with the updated Files slice
					currentConfigStruct := configs[currentConfig]
					currentConfigStruct.Files = append(currentConfigStruct.Files, filesList...)
					configs[currentConfig] = currentConfigStruct
				}
			} else if trimmedLine == "flags {" {
				inFlagsBlock = true
			} else if inFlagsBlock && strings.Contains(trimmedLine, "=") {
				parts := strings.SplitN(trimmedLine, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
					// Create a new Config struct with the updated Flags map
					currentConfigStruct := configs[currentConfig]
					currentConfigStruct.Flags[key] = value
					configs[currentConfig] = currentConfigStruct
				}
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
		indexFile.Configs[configName] = Config{Files: cleanedFiles, Flags: config.Flags}
	}
}
