package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type IndexFile struct {
	Version       int
	LastUpdated   string
	Configs       map[string]Config
	DefaultValues map[string]string
}

type Config struct {
	Files []string
	Flags map[string]string
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
		indexFile.Configs[configName] = Config{Files: cleanedFiles, Flags: config.Flags}
	}
}
