// package main

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
// 	"os"
// 	"os/exec"
// 	"path/filepath"
// 	"runtime"
// 	"strconv"
// 	"strings"
// 	"time"

// 	"github.com/charmbracelet/huh"
// 	"github.com/charmbracelet/lipgloss"
// 	"github.com/charmbracelet/log"
// 	"github.com/civo/civogo"
// 	"github.com/digitalocean/godo"
// 	"github.com/hashicorp/hcl/v2"
// 	"github.com/hashicorp/hcl/v2/hclsyntax"
// 	"github.com/hashicorp/hcl/v2/hclwrite"
// 	"github.com/zclconf/go-cty/cty"
// )
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
)

func main() {
	log.SetOutput(os.Stderr)
	printIntro()

	err := initializeAndCleanup()
	if err != nil {
		log.Error("Error initializing and cleaning up", "error", err)
		os.Exit(1)
	}

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

func initializeAndCleanup() error {
    indexFile, err := loadIndexFile()
    if err != nil {
        return err
    }
    cleanupIndexFile(&indexFile)
    
    // Create a new CloudConfig instance and pass its address
    config := NewCloudConfig()
    return updateIndexFile(&config, indexFile)
}
