package tui

import (
	"DockSTARTer2/internal/config"
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func TestLayoutMath() {
	cfg := config.LoadAppConfig()
	cfg.UI.Shadow = true
	currentConfig = cfg
	InitStyles(cfg)

	// override line chars for testing just in case
	cfg.UI.LineCharacters = false

	app := AppModel{
		ctx:    context.Background(),
		config: cfg,
	}

	app.width = 80
	app.height = 37

	app.backdrop = NewBackdropModel("")
	app.backdrop.SetSize(80, 37)

	app.logPanel = NewLogPanelModel()
	app.logPanel.expanded = false // Keep it closed like screenshot 1
	app.logPanel.SetSize(80, 37)

	// Create generic menu model
	dispScreen := NewMenuModel("test", "App Select", "", nil, nil)
	dispScreen.SetMaximized(true)
	app.dialog = &dispScreen

	app.Update(tea.WindowSizeMsg{Width: 80, Height: 37})
	viewStr := app.View().Content

	lines := strings.Split(viewStr, "\n")

	// Find where the ^ Log ^ is
	logY := -1
	for i, line := range lines {
		if strings.Contains(line, "^ Log ^") {
			logY = i
		}
	}

	// Find where the shadow is
	shadowY := -1
	for i, line := range lines {
		if strings.Contains(line, "▒") {
			shadowY = i
		}
	}

	// Find the bottom border of the dialog
	borderY := -1
	for i, line := range lines {
		if strings.Contains(line, "\\") || strings.Contains(line, "╰") || strings.Contains(line, "+") || strings.Contains(line, "╯") || strings.Contains(line, "/") {
			// It might be the actual bottom box
			if strings.Contains(line, "---") || strings.Contains(line, "===") || strings.Contains(line, "___") || strings.Contains(line, "━━") || strings.Contains(line, "───") {
				borderY = i
			}
		}
	}

	fmt.Printf("Total View Lines (including empty strings from Split): %d\n", len(lines))
	fmt.Printf("Terminal Height: 37\n")
	fmt.Printf("ContentArea Height inside AppModel: %d\n", app.backdropHeight()-5) // roughly
	fmt.Printf("Last Border found at Y: %d\n", borderY)
	fmt.Printf("Shadow found starting at Y: %d\n", shadowY)
	fmt.Printf("Log strip found at Y: %d\n", logY)

	if shadowY >= logY-1 {
		fmt.Printf("OVERLAP DETECTED! Shadow at %d, LogStrip at %d (Helpline at %d)\n", shadowY, logY, logY-1)
	} else {
		fmt.Printf("NO OVERLAP. Shadow at %d, LogStrip at %d (Helpline at %d)\n", shadowY, logY, logY-1)
	}

	for i := shadowY - 2; i <= logY && i >= 0 && i < len(lines); i++ {
		fmt.Printf("%02d: %s\n", i, GetPlainText(lines[i]))
	}
	os.WriteFile("/tmp/test_out.txt", []byte(viewStr), 0644)
}
