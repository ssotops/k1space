package main

import (
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
)

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

func runConfigMenu() {
	for {
		var selected string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Config Menu").
					Options(
						huh.NewOption("List Configs", "List Configs"),
						huh.NewOption("Create Config", "Create Config"),
						huh.NewOption("Delete Config", "Delete Config"),
						huh.NewOption("Delete All Configs", "Delete All Configs"),
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
		case "List Configs":
			listConfigs()
		case "Create Config":
			createConfig(&CloudConfig{})
		case "Delete Config":
			deleteConfig()
		case "Delete All Configs":
			deleteAllConfigs()
		case "Back":
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
						huh.NewOption("Run Kubefirst Repositories", "Run Kubefirst Repositories"),
						huh.NewOption("Revert to Main", "Revert to Main"),
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
			runKubefirstSetup()
		case "Run Kubefirst Repositories":
			runKubefirstRepositories()
		case "Revert to Main":
			revertKubefirstToMain()
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
