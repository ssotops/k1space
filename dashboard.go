package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	titleStyle = lipgloss.NewStyle().
			Foreground(special).
			Padding(0, 1).
			MarginBottom(1).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(subtle).
			Padding(1)

	docStyle = lipgloss.NewStyle().Padding(1, 2, 1, 2)

	// Define styles for each service
	consoleStyle      = infoStyle.Copy().BorderForeground(lipgloss.Color("#00FFFF"))
	kubefirstAPIStyle = infoStyle.Copy().BorderForeground(lipgloss.Color("#FF00FF"))
	kubefirstStyle    = infoStyle.Copy().BorderForeground(lipgloss.Color("#FFFF00"))
)

func renderDashboard(kubefirstAPILogs, consoleLogs, kubefirstLogs []string, summary []string) string {
	width := 120
	height := 30

	doc := strings.Builder{}

	// Summary
	summaryStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(highlight).
		Padding(1).
		Width(width - 4)

	summaryContent := "Summary:\n" + strings.Join(summary, "\n")
	doc.WriteString(summaryStyle.Render(summaryContent))
	doc.WriteString("\n\n")

	// Create styles for each component
	componentStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(subtle).
		Padding(1).
		Width((width-8)/3 - 1).
		Height(height)

	// Render each component
	kubefirstAPIInfo := componentStyle.Render(fmt.Sprintf("Kubefirst-API Logs:\n%s", strings.Join(kubefirstAPILogs, "\n")))
	consoleInfo := componentStyle.Render(fmt.Sprintf("Console Logs:\n%s", strings.Join(consoleLogs, "\n")))
	kubefirstInfo := componentStyle.Render(fmt.Sprintf("Kubefirst Logs:\n%s", strings.Join(kubefirstLogs, "\n")))

	// Combine components horizontally
	row := lipgloss.JoinHorizontal(lipgloss.Top, kubefirstAPIInfo, consoleInfo, kubefirstInfo)

	doc.WriteString(row)

	return docStyle.Render(doc.String())
}
