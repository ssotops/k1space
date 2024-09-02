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
		configBody := configBlock.Body()

		fileValues := make([]cty.Value, len(v.Files))
		for i, file := range v.Files {
			fileValues[i] = cty.StringVal(file)
		}
		configBody.SetAttributeValue("files", cty.ListVal(fileValues))

		flagsBlock := configBody.AppendNewBlock("flags", nil)
		flagsBody := flagsBlock.Body()
		for flagK, flagV := range v.Flags {
			flagsBody.SetAttributeValue(flagK, cty.StringVal(flagV))
		}
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

		// Read the .local.cloud.env file
		envFilePath := filepath.Join(os.Getenv("HOME"), ".ssot", "k1space", strings.ToLower(config.CloudPrefix), strings.ToLower(config.Region), config.StaticPrefix, ".local.cloud.env")
		envContent, err := os.ReadFile(envFilePath)
		if err != nil {
			return fmt.Errorf("error reading .local.cloud.env: %w", err)
		}

		// Parse the environment variables
		envVars := strings.Split(string(envContent), "\n")
		for _, envVar := range envVars {
			if strings.TrimSpace(envVar) == "" {
				continue
			}
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}
			flagName := strings.TrimPrefix(parts[0], "export K1_"+strings.ToUpper(config.CloudPrefix)+"_"+strings.ToUpper(config.Region)+"_")
			flagValue := strings.Trim(parts[1], "\"")
			newConfig.Flags[strings.ToLower(flagName)] = flagValue
		}

		// Update or add the new configuration
		indexFile.Configs[key] = newConfig
	}

	// Add this new section here
	for key := range indexFile.Configs {
		parts := strings.Split(key, "_")
		if len(parts) != 3 {
			// Remove invalid configs
			delete(indexFile.Configs, key)
		}
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
	nestedLevel := 0

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "configs {" {
			inConfigsBlock = true
			nestedLevel++
			continue
		}
		if inConfigsBlock {
			if strings.HasSuffix(trimmedLine, "{") {
				nestedLevel++
				if nestedLevel == 2 {
					currentConfig = strings.TrimSuffix(trimmedLine, " {")
					configs[currentConfig] = Config{Files: []string{}, Flags: make(map[string]string)}
				} else if nestedLevel == 3 && trimmedLine == "flags {" {
					inFlagsBlock = true
				}
			} else if trimmedLine == "}" {
				nestedLevel--
				if nestedLevel == 1 {
					currentConfig = ""
					inFlagsBlock = false
				} else if nestedLevel == 0 {
					inConfigsBlock = false
				}
			} else if strings.HasPrefix(trimmedLine, "files = [") {
				files := strings.Trim(strings.TrimPrefix(trimmedLine, "files = ["), "]")
				if files != "" && currentConfig != "" {
					filesList := strings.Split(files, ", ")
					for i := range filesList {
						filesList[i] = strings.Trim(filesList[i], "\"")
					}
					currentConfigStruct := configs[currentConfig]
					currentConfigStruct.Files = append(currentConfigStruct.Files, filesList...)
					configs[currentConfig] = currentConfigStruct
				}
			} else if inFlagsBlock && strings.Contains(trimmedLine, "=") {
				parts := strings.SplitN(trimmedLine, "=", 2)
				if len(parts) == 2 && currentConfig != "" {
					key := strings.TrimSpace(parts[0])
					value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
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
