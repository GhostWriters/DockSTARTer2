package screens

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
)

// DisplayOptionsScreen allows the user to configure UI settings and themes together.
type DisplayOptionsScreen struct {
	loadDefaultsMenu *displayengine.MenuModel
	themeMenu        *displayengine.MenuModel
	optionsMenu      *displayengine.MenuModel // the shown connection type's options
	tintControlsMenu *displayengine.MenuModel // Tint pane: element choice and on/off switches
	tintMenu         *displayengine.MenuModel // Tint pane scheme list
	panes            *displayengine.TabbedPanes
	isRoot           bool // true when launched directly via -M appearance; hides Back button

	config       config.AppConfig
	themes       []theme.ThemeMetadata
	currentTheme string
	previewTheme string // Theme currently being highlighted in the list

	width  int
	height int

	outerMenu *displayengine.MenuModel // outer "Appearance Settings" dialog with sections + buttons
	layoutRow *appearanceLayoutRow     // pairs the settings column with the preview section

	// previewViewport/previewScroll hold the preview panel's scroll position
	// across renders. buildPreviewSection rebuilds the mockup's rendered
	// content fresh every render (to stay live), but reuses these persistent
	// fields so scrolling isn't reset to the top each time -- SetContent
	// preserves viewport.Model's YOffset (only clamping it if it's now past
	// the new content's end), so this works as long as the same instances
	// are threaded through instead of constructing fresh ones per render.
	previewViewport viewport.Model
	previewScroll   displayengine.Scrollbar

	focused bool // tracks global screen focus (header/log panel interaction)

	baseConfig     config.AppConfig                // Original exact config before previewing
	themeDefaults  map[string]*theme.ThemeDefaults // Cache parsed defaults
	themeFileCache map[string]theme.ThemeFile      // Cache GetThemeFile results for help text

	// loadThemeDefaults controls whether focusing a theme in the list stages
	// that theme's own [defaults] table on top of the current options. This is
	// a screen-local preference, not part of config.AppConfig -- it only
	// affects behavior while this screen is open, not anything persisted.
	loadThemeDefaults bool

	// themeChangedFields holds the config.Appearance struct field names whose
	// value the most recent applyPreview call actually changed via the
	// theme's own [defaults] table. Drives the transient "changed" marker
	// (same glyph/tag as App Select's just-added/renamed marker) shown in
	// front of the corresponding Options row until the next interaction.
	themeChangedFields map[string]bool

	// connType is the session's own connection type; editType is the one
	// whose tab is shown. unlocked reports whether this session may edit
	// other connection types' tabs: always for Local, and for a remote
	// session once it passes the sudo gate, until the screen closes.
	connType string
	editType string
	unlocked bool

	tabs  *displayengine.TabStrip
	frame *tabFrame

	// tintElement is the tint element ("menu", "programbox", "cli") the Tint
	// pane shows; tintCatalog the schemes it offers.
	tintElement     string
	tintCatalog     []commands.TintEntry
	tintDownloading bool

	// previewTintKey is the tint key the preview renders under, holding the
	// shown tab's staged Menu tint (previewTint) -- private to this screen,
	// so the rest of the screen keeps the live tint until Apply.
	previewTintKey string
	previewTint    *config.AnsiElementColors
}

// toggleLoadThemeDefaultsMsg flips loadThemeDefaults. Handled directly rather
// than via updateDisplayOptionMsg since it's not a config.AppConfig field.
type toggleLoadThemeDefaultsMsg struct{}

// updateDisplayOptionMsg is sent when an option is changed in the menu
type updateDisplayOptionMsg struct {
	update func(*config.AppConfig)
}

// tabUnlockedMsg reports that the sudo gate passed for connType's tab;
// focusFrame is switchTab's.
type tabUnlockedMsg struct {
	connType   string
	focusFrame bool
}

// displayOptionsAbortMsg is sent when Apply is attempted but blocked (e.g. command lock).
// Handled by Update to clear the processing spinner without applying changes.
type displayOptionsAbortMsg struct{}

// NewDisplayOptionsScreen creates a new consolidated display options screen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewDisplayOptionsScreen(isRoot bool, connType string) *DisplayOptionsScreen {
	cfg := config.LoadAppConfig()
	current := cfg.Appearance.ForConnType(connType).Theme // ConfigValue e.g. "DockSTARTer" or "user:MyTheme"
	themes, _ := theme.List(current)

	s := &DisplayOptionsScreen{
		isRoot:            isRoot,
		connType:          connType,
		editType:          connType,
		unlocked:          connType == "local",
		config:            cfg,
		baseConfig:        cfg,
		themes:            themes,
		currentTheme:      current,
		previewTheme:      current,
		themeDefaults:     make(map[string]*theme.ThemeDefaults),
		themeFileCache:    make(map[string]theme.ThemeFile),
		loadThemeDefaults: true,
		tintElement:       "menu",
		previewViewport:   viewport.New(),
		// Must match mockupMenu's own ID in buildPreviewSection exactly --
		// MatchesID checks msgID.Contains(m.ID()), so the scrollbar's hit
		// region IDs ("<ID>.sb.*") need mockupMenu's ID as a substring or its
		// clicks/hovers never resolve to the preview section at all.
		previewScroll: displayengine.Scrollbar{ID: "appearance_preview_mockup"},
	}
	labels := make([]string, len(config.ConnTypes))
	for i, ct := range config.ConnTypes {
		labels[i] = config.ConnTypeLabel(ct)
		if ct == connType {
			labels[i] += " (Current)"
		}
	}
	s.tabs = &displayengine.TabStrip{ID: "appearance_conntype", Labels: labels, Active: connTypeIndex(connType)}
	s.tabs.Changed = func(i int) bool { return s.tabChanged(config.ConnTypes[i]) }
	s.frame = &tabFrame{strip: s.tabs, focused: s.frameFocused}
	// Without this, bubbles/viewport only renders as many rows as the
	// content actually has -- any content shorter than the assigned height
	// (even by one line, e.g. from an off-by-one in the backdrop's own
	// fill-to-height math) leaves an unstyled gap at the bottom instead of
	// the viewport padding it out itself.
	s.previewViewport.FillHeight = true
	s.themeDefaults[current], _ = theme.Load(current, "Preview")
	s.loadTintCatalog()
	s.previewTintKey = fmt.Sprintf("ds2-preview-tint-%p", s)

	s.initMenus()
	s.focused = true // Default to focused initially
	return s
}

func (s *DisplayOptionsScreen) initMenus() {
	selected := s.config.Appearance.ForConnType(s.editType).Theme

	// 1. Theme Selection Menu, grouped by source (see groupedItems).
	var bundledThemes, userThemes, otherThemes []displayengine.MenuItem
	foundCurrent := false
	for _, t := range s.themes {
		desc := t.Description
		if t.Author != "" {
			desc += fmt.Sprintf(" [by %s]", t.Author)
		}
		descTag := "{{|ItemList|}}"
		if t.IsUserTheme {
			descTag = "{{|ItemListUserDefined|}}"
		}
		checked := selected == t.ConfigValue
		if checked {
			foundCurrent = true
		}
		item := displayengine.MenuItem{
			Tag:           t.Name,
			Desc:          descTag + desc,
			Help:          desc,
			IsRadioButton: true,
			Selectable:    true,
			Checked:       checked,
			IsInvalid:     t.IsInvalid,
			IsUserDefined: t.IsUserTheme,
			Metadata:      map[string]string{"config_value": t.ConfigValue},
		}
		if t.IsUserTheme {
			userThemes = append(userThemes, item)
		} else {
			bundledThemes = append(bundledThemes, item)
		}
	}
	// file: themes point outside the themes folder and are never part of
	// s.themes (theme.List only enumerates embedded and user: themes), so
	// foundCurrent is never true for one -- check the file directly instead
	// of treating "not in the list" as "missing".
	if !foundCurrent && strings.HasPrefix(selected, "file:") {
		if _, err := os.Stat(strings.TrimPrefix(selected, "file:")); err == nil {
			otherThemes = append(otherThemes, displayengine.MenuItem{
				Tag:           "file:" + theme.ThemeDisplayName(selected),
				Desc:          "{{|ItemListUserDefined|}}External theme file",
				Help:          "Theme loaded directly from a file outside the themes folder.",
				IsRadioButton: true,
				Selectable:    true,
				Checked:       true,
				IsUserDefined: true,
				Metadata:      map[string]string{"config_value": selected},
			})
			foundCurrent = true
		}
	}
	// If the configured theme still doesn't match anything (its file was
	// removed, or it's a user:/embedded reference no longer on disk),
	// prepend a placeholder so the user can see what is active and
	// optionally switch away from it.
	if !foundCurrent && selected != "" {
		shortURI := selected
		if strings.HasPrefix(selected, "file:") {
			shortURI = "file:" + theme.ThemeDisplayName(selected)
		}
		displayName := "(missing) " + shortURI
		otherThemes = append(otherThemes, displayengine.MenuItem{
			Tag:           displayName,
			Desc:          "{{|ItemListUserDefined|}}Source file not found — using cached version",
			Help:          "Theme source file is missing. The cached version remains active until you choose another theme.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       true,
			IsUserDefined: true,
			Metadata:      map[string]string{"config_value": selected},
		})
	}
	themeItems := groupedItems([]listGroup{
		{Label: "Current", Items: otherThemes},
		{Label: "Bundled", Items: bundledThemes},
		{Label: "User", Items: userThemes},
	})

	themeMenu := displayengine.NewMenuModel(displayengine.IDThemePanel, config.ConnTypeLabel(s.editType)+" Theme", "", themeItems)
	s.themeMenu = themeMenu
	s.themeMenu.SetHelpItemPrefix("Theme")
	s.themeMenu.SetRowCache(true)
	s.themeMenu.SetItemHelpFunc(s.buildThemeItemHelp)
	s.themeMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.themeMenu.SetSubMenuMode(true)
	s.themeMenu.SetVariableHeight(true)
	s.themeMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.themeMenu.SetButtons([]displayengine.ButtonDef{})
	s.themeMenu.SetMaximized(true) // Fill available width
	s.themeMenu.SetShowLockGutter(false)
	s.themeMenu.SetNoLeftMargin(true)

	// 2. Load Theme Defaults Menu (own section, above the theme list, since
	// toggling it affects how focusing a theme below behaves)
	loadDefaultsItems := []displayengine.MenuItem{
		{
			Tag:        "Load Theme Defaults",
			Desc:       "Stage a theme's suggested options when focused",
			Help:       "When on, browsing themes stages that theme's own suggested options below (Space to toggle)",
			IsCheckbox: true,
			Checked:    s.loadThemeDefaults,
			Selectable: true,
			SpaceAction: func() tea.Msg {
				return toggleLoadThemeDefaultsMsg{}
			},
		},
	}
	loadDefaultsMenu := displayengine.NewMenuModel(displayengine.IDLoadDefaultsPanel, "", "", loadDefaultsItems)
	s.loadDefaultsMenu = loadDefaultsMenu
	s.loadDefaultsMenu.SetHelpItemPrefix("Option")
	s.loadDefaultsMenu.SetHelpPageText("Controls whether focusing a theme below stages that theme's own suggested options.")
	s.loadDefaultsMenu.SetSubMenuMode(true)
	s.loadDefaultsMenu.SetIsDialog(false)
	s.loadDefaultsMenu.SetButtons([]displayengine.ButtonDef{})
	s.loadDefaultsMenu.SetFlowMode(true)
	s.loadDefaultsMenu.SetMaximized(true)
	s.loadDefaultsMenu.SetShowLockGutter(false)

	// 3. Options Menu
	// Grouped: visual toggles, then brackets, then title alignment, then performance.
	optionItems := []displayengine.MenuItem{
		// -- Visual toggles --
		{
			Tag:         "Borders",
			Desc:        "Show borders on all dialogs",
			Help:        "Toggle border visibility (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).Borders,
			Selectable:  true,
			SpaceAction: s.toggleBorders(),
		},
		{
			Tag:         "Large Buttons",
			Desc:        "Show large (bordered) buttons",
			Help:        "Toggle large (bordered) vs flat button style (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).LargeButtons,
			Selectable:  true,
			SpaceAction: s.toggleLargeButtons(),
		},
		{
			Tag:         "Large Title Bars",
			Desc:        "Show title in a separate row above content",
			Help:        "Toggle large title bar style (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).LargeTitleBars,
			Selectable:  true,
			SpaceAction: s.toggleLargeTitleBars(),
		},
		{
			Tag:         "Line Characters",
			Desc:        "Use unicode line drawing characters",
			Help:        "Use ┌─ instead of +- for borders (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).LineCharacters,
			Selectable:  true,
			SpaceAction: s.toggleLineChars(),
		},
		{
			Tag:         "Shadows",
			Desc:        "Enable drop shadows",
			Help:        "Toggle drop shadow effect (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).Shadow,
			Selectable:  true,
			SpaceAction: s.toggleShadow(),
		},
		{
			Tag:    "Shadow Level",
			Desc:   s.dropdownDesc(s.shadowLevelToDesc(s.config.Appearance.Ptr(s.editType).ShadowLevel)),
			Help:   "Adjust the density of the shadow (Select/Enter for list)",
			Action: s.showShadowDropdown(),
		},
		{
			Tag:    "Border Color",
			Desc:   s.dropdownDesc(s.borderColorToDesc(s.config.Appearance.Ptr(s.editType).BorderColor)),
			Help:   "Choose theme colors for borders (Select/Enter for list)",
			Action: s.showBorderColorDropdown(),
		},
		{
			Tag:         "Scrollbars",
			Desc:        "Show scrollbar in lists",
			Help:        "Toggle scrollbar in scrollable lists (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).Scrollbar,
			Selectable:  true,
			SpaceAction: s.toggleScrollbar(),
		},
		{
			Tag:         "Spinners",
			Desc:        "Show loading spinner animations",
			Help:        "Toggle spinner animations during loading (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).Spinner,
			Selectable:  true,
			SpaceAction: s.toggleSpinner(),
		},
		{
			Tag:         "Show Preview",
			Desc:        "Show the preview panel by default (some prefer a less busy screen)",
			Help:        "Default visibility of this preview panel (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).ShowPreview,
			Selectable:  true,
			SpaceAction: s.toggleShowPreview(),
		},
		{
			Tag:  "Theme/Tint Layout",
			Desc: s.dropdownDesc(tabLayoutDesc(s.config.Appearance.Ptr(s.editType).PaneLayout)),
			Help: "Default layout of the Theme and Tint panes; Ctrl+W switches it while here (Enter for options)",
			Action: s.showTabLayoutDropdown("pane_layout", "Theme/Tint Layout",
				func() string { return s.config.Appearance.Ptr(s.editType).PaneLayout },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).PaneLayout = v }),
		},
		{
			Tag:         "Menu Brackets",
			Desc:        "Wrap the focused menu item in [brackets]",
			Help:        "Bracket the focused row's tag (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).MenuBrackets,
			Selectable:  true,
			SpaceAction: s.toggleMenuBrackets(),
		},
		{
			Tag:         "Line Number Brackets",
			Desc:        "Wrap the focused line number in [brackets]",
			Help:        "Bracket the focused line's number in the env editor (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Appearance.Ptr(s.editType).LineNumberBrackets,
			Selectable:  true,
			SpaceAction: s.toggleLineNumberBrackets(),
		},
		{
			Tag:  "Tab Layout",
			Desc: s.dropdownDesc(tabLayoutDesc(s.config.Appearance.Ptr(s.editType).TabLayout)),
			Help: "Default view when the vars editor has 2 tabs open (Enter for options)",
			Action: s.showTabLayoutDropdown("tab_layout", "Tab Layout",
				func() string { return s.config.Appearance.Ptr(s.editType).TabLayout },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).TabLayout = v }),
		},

		// -- Brackets --
		{
			Tag:  "Checkbox Brackets",
			Desc: s.dropdownDesc(bracketModeDesc(s.config.Appearance.Ptr(s.editType).CheckboxBrackets)),
			Help: "When checkbox brackets are shown in lists (Enter for options)",
			Action: s.showBracketModeDropdown("checkbox_brackets", "Checkbox Brackets",
				func() string { return s.config.Appearance.Ptr(s.editType).CheckboxBrackets },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).CheckboxBrackets = v }),
		},
		{
			Tag:  "Radio Brackets",
			Desc: s.dropdownDesc(bracketModeDesc(s.config.Appearance.Ptr(s.editType).RadioBrackets)),
			Help: "When radio button brackets are shown in lists (Enter for options)",
			Action: s.showBracketModeDropdown("radio_brackets", "Radio Brackets",
				func() string { return s.config.Appearance.Ptr(s.editType).RadioBrackets },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).RadioBrackets = v }),
		},

		// -- Title alignment --
		{
			Tag:  "Dialog Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).DialogTitleAlign)),
			Help: "Alignment of titles in dialog borders (Enter for options)",
			Action: s.showTitleAlignDropdown("dialog_title_align", "Dialog Title Align",
				func() string { return s.config.Appearance.Ptr(s.editType).DialogTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).DialogTitleAlign = v }),
		},
		{
			Tag:  "Submenu Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).SubmenuTitleAlign)),
			Help: "Alignment of subtitle rows inside menus (Enter for options)",
			Action: s.showTitleAlignDropdown("submenu_title_align", "Submenu Title Align",
				func() string { return s.config.Appearance.Ptr(s.editType).SubmenuTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).SubmenuTitleAlign = v }),
		},
		{
			Tag:  "Panel Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).PanelTitleAlign)),
			Help: "Alignment of the panel strip label (Enter for options)",
			Action: s.showTitleAlignDropdown("panel_title_align", "Panel Title Align",
				func() string { return s.config.Appearance.Ptr(s.editType).PanelTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).PanelTitleAlign = v }),
		},

		// -- Panel, links, and timing --
		{
			Tag:           "Panel Mode",
			Desc:          s.dropdownDesc(s.panelModeToDesc(s.config.Appearance.Ptr(s.editType).Panel)),
			Help:          "Choose the panel shown below the menus (Enter for options)",
			Action:        s.showPanelDropdown(),
			IsDestructive: true,
		},
		{
			Tag:  "Hyperlinks",
			Desc: s.dropdownDesc(hyperlinksDesc(s.config.Appearance.Ptr(s.editType).Hyperlinks)),
			Help: "OSC8 hyperlink rendering for DS2's own console/path/link tags (Enter for options)",
			Action: s.showMarkdownHyperlinksDropdown("hyperlinks", "Hyperlinks",
				func() string { return s.config.Appearance.Ptr(s.editType).Hyperlinks },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).Hyperlinks = v }),
		},
		{
			Tag:  "Markdown Hyperlinks",
			Desc: s.dropdownDesc(markdownHyperlinksDesc(s.config.Appearance.Ptr(s.editType).MarkdownHyperlinks)),
			Help: "OSC8 hyperlink rendering for markdown, e.g. the help dialog doc page and --man (Enter for options)",
			Action: s.showMarkdownHyperlinksDropdown("markdown_hyperlinks", "Markdown Hyperlinks",
				func() string { return s.config.Appearance.Ptr(s.editType).MarkdownHyperlinks },
				func(cfg *config.AppConfig, v string) { cfg.Appearance.Ptr(s.editType).MarkdownHyperlinks = v }),
		},
		{
			Tag:        "Refresh Rate",
			Desc:       fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", s.config.Appearance.Ptr(s.editType).RefreshRate),
			Help:       "Screen repaint interval in milliseconds (Enter to change). Applies on restart.",
			Action:     s.promptRefreshRate(),
			Selectable: true,
		},
		{
			Tag:        "Spinner Speed",
			Desc:       fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", s.config.Appearance.Ptr(s.editType).SpinnerSpeed),
			Help:       "Spinner frame speed in milliseconds (Enter to change)",
			Action:     s.promptSpinnerSpeed(),
			Selectable: true,
		},
	}

	optionsMenu := displayengine.NewMenuModel(displayengine.IDOptionsPanel, config.ConnTypeLabel(s.editType)+" Options", "", optionItems)
	s.optionsMenu = optionsMenu
	s.optionsMenu.SetHelpItemPrefix("Option")
	s.optionsMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.optionsMenu.SetSubMenuMode(true)
	s.optionsMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.optionsMenu.SetButtons([]displayengine.ButtonDef{})
	s.optionsMenu.SetFlowMode(true)
	s.optionsMenu.SetMaximized(true) // Fill available width
	s.optionsMenu.SetShowLockGutter(true)

	// 4. Outer "Appearance Settings" dialog (sections container + buttons)
	outerMenu := displayengine.NewMenuModel("appearance_outer", "Appearance Settings", "", nil)
	applyAction := func() tea.Msg { return s.handleApply()() }
	resetAction := func() tea.Msg { return s.handleReset()() }
	if s.isRoot {
		outerMenu.SetButtons([]displayengine.ButtonDef{
			{Label: "Apply", ZoneID: displayengine.IDApplyButton, Action: applyAction, Help: "Apply and save appearance settings."},
			{Label: "Reset", ZoneID: displayengine.IDResetButton, Action: resetAction, Help: "Discard staged changes and revert to the current saved settings."},
			{Label: "Exit", ZoneID: displayengine.IDExitButton, Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	} else {
		outerMenu.SetButtons([]displayengine.ButtonDef{
			{Label: "Apply", ZoneID: displayengine.IDApplyButton, Action: applyAction, Help: "Apply and save appearance settings."},
			{Label: "Reset", ZoneID: displayengine.IDResetButton, Action: resetAction, Help: "Discard staged changes and revert to the current saved settings."},
			{Label: "Back", ZoneID: displayengine.IDBackButton, Action: navigateBack(), Help: "Return to the previous screen."},
			{Label: "Exit", ZoneID: displayengine.IDExitButton, Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	}
	// Title-bar refresh icon mirrors the Reset button, matching the tabbed
	// vars editor's use of the same widget for its own reload action. Extra
	// widgets go before Help/Close, which stay rightmost by convention (see
	// TitleBarFocus doc comment).
	outerMenu.ConfigureWidgets(displayengine.WidgetRefresh, displayengine.WidgetHelp, displayengine.WidgetClose)
	// Apply/Reset/Back/Exit are real, unrelated buttons -- an Options
	// dropdown (or any other nested item) starting its own deferred action
	// has no business visually activating Apply, unlike a simple
	// single-list dialog where an item click IS conceptually "press Select".
	outerMenu.SetSuppressChildProcessingMark(true)
	// Without this, calculateSectionLayout's "!m.maximized" pass shrinks the
	// whole dialog down to its content sections' summed natural heights
	// instead of filling the space the screen actually has available --
	// unlike a small popup dialog, this is a full-screen settings surface
	// (every child section below already calls SetMaximized(true) for the
	// same reason) and should always claim the height it's given.
	outerMenu.SetMaximized(true)
	// Theme and Tint panes (see TabbedPanes), keeping the layout and shown
	// pane across rebuilds.
	layout, shown := s.baseConfig.Appearance.Ptr(s.connType).PaneLayout, 0
	if s.panes != nil {
		layout, shown = s.panes.Layout(), s.panes.Active()
	}
	s.buildTintMenus()
	s.panes = displayengine.NewTabbedPanes("appearance_panes", []string{"Theme", "Tint"},
		[]*displayengine.ContentColumn{
			displayengine.NewContentColumn(loadDefaultsMenu, themeMenu),
			displayengine.NewContentColumn(s.tintControlsMenu, s.tintMenu),
		}, layout)
	s.panes.Strip.Active = shown
	settingsColumn := displayengine.NewContentColumn(
		newTabFrameSection(s.panes, s.frame, true, false),
		newTabFrameSection(optionsMenu, s.frame, false, true),
	)
	previewHidden := !s.config.Appearance.Ptr(s.connType).ShowPreview
	if s.layoutRow != nil {
		previewHidden = s.layoutRow.previewHidden
	}
	s.layoutRow = newAppearanceLayoutRow(settingsColumn, s.buildPreviewSection())
	s.layoutRow.previewHidden = previewHidden
	outerMenu.AddContentSection(s.layoutRow)
	s.outerMenu = outerMenu
	s.refreshPreviewTint()
}

// refreshPreviewTint registers the shown tab's staged Menu tint under
// previewTintKey when it has changed.
func (s *DisplayOptionsScreen) refreshPreviewTint() {
	el := s.config.Appearance.ForConnType(s.editType).AnsiColors.Element("menu")
	if s.previewTint != nil && reflect.DeepEqual(*s.previewTint, el) {
		return
	}
	s.previewTint = &el
	tui.RegisterTintKey(context.Background(), s.previewTintKey, el)
}

// focusedSettingsMenu returns whichever of loadDefaultsMenu/themeMenu/
// optionsMenu currently holds section-internal focus, or nil when focus is
// elsewhere (buttons, or the preview side once it's a real Tab stop) --
// outerMenu/layoutRow track focus generically now (GetFocusedSection/
// GetFocusedItem, and the row's own subFocus/settings.SubFocusIndex), so
// this just reads that state instead of a separate parallel one.
func (s *DisplayOptionsScreen) focusedSettingsMenu() *displayengine.MenuModel {
	if s.outerMenu == nil || s.layoutRow == nil {
		return nil
	}
	if s.outerMenu.GetFocusedItem() != displayengine.FocusList || s.outerMenu.GetFocusedSection() != 0 {
		return nil
	}
	if s.layoutRow.subFocus == 1 {
		return nil
	}
	leaves := s.layoutRow.settings.Items()
	i := s.layoutRow.settings.SubFocusIndex()
	if i < 0 || i >= len(leaves) {
		return nil
	}
	c := leaves[i]
	for {
		if m, ok := c.(*displayengine.MenuModel); ok {
			return m
		}
		w, ok := c.(displayengine.ContentWrapper)
		if !ok {
			return nil
		}
		c = w.Unwrap()
	}
}

// frameFocused reports whether focus is inside the connection-type tab
// frame, which holds every settings section.
func (s *DisplayOptionsScreen) frameFocused() bool {
	return s.focusedSettingsMenu() != nil
}

// connTypeIndex returns connType's position in config.ConnTypes (0 if unknown).
func connTypeIndex(connType string) int {
	for i, ct := range config.ConnTypes {
		if ct == connType {
			return i
		}
	}
	return 0
}

// switchTab shows connType's tab, keeping the focused rows. focusFrame
// (a tab click) moves focus into the tab frame, to the section last focused
// there; otherwise focus stays where it is, preview and buttons included. A Local session may edit every tab; an SSH or Web Server session must
// pass the sudo gate once before leaving its own tab. Staged changes on
// every tab are kept.
func (s *DisplayOptionsScreen) switchTab(connType string, focusFrame bool) tea.Cmd {
	if connType == s.editType {
		if focusFrame {
			return s.focusFrame()
		}
		return nil
	}
	if !s.unlocked && connType != s.connType {
		return s.unlockTab(connType, focusFrame)
	}
	focusIdx, rowFocus := 0, 0
	if s.layoutRow != nil {
		focusIdx, rowFocus = s.layoutRow.settings.SubFocusIndex(), s.layoutRow.subFocus
	}
	onButtons := s.outerMenu != nil && s.outerMenu.GetFocusedItem() != displayengine.FocusList
	btnIdx := 0
	if onButtons {
		btnIdx = s.outerMenu.GetFocusedBtnIndex()
	}
	optionCursor := s.optionsMenu.Index()
	focusedTheme := s.themeMenu.SelectedItem()
	s.editType = connType
	s.tabs.Active = connTypeIndex(connType)
	s.currentTheme = s.baseConfig.Appearance.ForConnType(connType).Theme
	s.previewTheme = s.config.Appearance.ForConnType(connType).Theme
	s.themeChangedFields = nil
	s.themeDefaults[s.previewTheme], _ = theme.Load(s.previewTheme, "Preview")
	displayengine.ClearSemanticCachePrefix("Preview_")
	s.initMenus()
	// The options list is the same on every tab, so the same row stays
	// focused. In the theme list, a cursor on the checked theme moves to the
	// new tab's checked theme; any other theme stays focused.
	s.optionsMenu.Select(optionCursor)
	for i, it := range s.themeMenu.GetItems() {
		if it.IsSeparator {
			continue
		}
		if focusedTheme.Checked && it.Checked || !focusedTheme.Checked && itemConfigValue(it) == itemConfigValue(focusedTheme) {
			s.themeMenu.Select(i)
			break
		}
	}
	s.layoutRow.settings.SetSubFocusIndex(focusIdx)
	s.layoutRow.subFocus = rowFocus
	if s.outerMenu != nil {
		s.outerMenu.SetFocused(s.focused)
	}
	s.SetSize(s.width, s.height)
	// The rebuilt menus still show whichever section held focus when they
	// were built; point that at the restored one. After SetSize, since the
	// preview can only hold focus once the row knows it fits.
	if focusFrame {
		return s.focusFrame()
	}
	if onButtons {
		s.outerMenu.SetFocusedBtnIndex(btnIdx)
		return s.layoutRow.SetSubFocused(false)
	}
	return s.layoutRow.SetSubFocused(true)
}

// focusFrame moves focus into the tab frame, to the section last focused
// there.
func (s *DisplayOptionsScreen) focusFrame() tea.Cmd {
	if s.layoutRow == nil || s.outerMenu == nil {
		return nil
	}
	s.layoutRow.subFocus = 0
	s.outerMenu.SetFocusedSection(0)
	return s.layoutRow.SetSubFocused(true)
}

// cycleTab moves to the previous (-1) or next (1) connection type's tab.
func (s *DisplayOptionsScreen) cycleTab(delta int) tea.Cmd {
	n := len(config.ConnTypes)
	return s.switchTab(config.ConnTypes[(connTypeIndex(s.editType)+delta+n)%n], false)
}

// unlockTab asks for the sudo password before a remote session may edit
// other connection types' settings, then switches to connType.
func (s *DisplayOptionsScreen) unlockTab(connType string, focusFrame bool) tea.Cmd {
	return func() tea.Msg {
		pass, err := tui.PromptText("Sudo Authentication",
			"Password required to edit other connection types' appearance settings:", true)
		if err != nil {
			if err == console.ErrUserAborted {
				return nil
			}
			return tui.ShowMessageDialogMsg{Title: "Authentication Error", Message: err.Error(), Type: tui.MessageError}
		}
		if msg := verifySudoPassword(pass); msg != "" {
			return tui.ShowMessageDialogMsg{Title: "Authentication Failed", Message: msg, Type: tui.MessageError}
		}
		return tabUnlockedMsg{connType: connType, focusFrame: focusFrame}
	}
}

// verifySudoPassword checks pass with "sudo -S -v", returning "" on success
// or a message describing the failure.
func verifySudoPassword(pass string) string {
	cmd := exec.Command("sudo", "-S", "-v")
	cmd.Stdin = strings.NewReader(pass + "\n")
	if err := cmd.Run(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return "sudo command not found on this system"
		}
		return "sudo: authentication failed"
	}
	return ""
}

// SetFocused updates the global focus state for this screen.
func (s *DisplayOptionsScreen) SetFocused(f bool) {
	s.focused = f
	if s.outerMenu != nil {
		s.outerMenu.SetFocused(f)
	}
}

// getThemeFile returns a cached ThemeFile for the given config value.
func (s *DisplayOptionsScreen) getThemeFile(configValue string) theme.ThemeFile {
	if tf, ok := s.themeFileCache[configValue]; ok {
		return tf
	}
	tf, _ := theme.GetThemeFile(configValue)
	s.themeFileCache[configValue] = tf
	return tf
}

// formatThemeDefaults produces a human-readable list of defaults a theme will apply.
// Returns an empty string when no defaults are set.
func formatThemeDefaults(d *theme.ThemeDefaults) string {
	if d == nil {
		return ""
	}
	boolStr := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	var lines []string
	if d.Borders != nil {
		lines = append(lines, fmt.Sprintf("  Borders: %s", boolStr(*d.Borders)))
	}
	if d.LargeButtons != nil {
		lines = append(lines, fmt.Sprintf("  Large Buttons: %s", boolStr(*d.LargeButtons)))
	}
	if d.LargeTitleBars != nil {
		lines = append(lines, fmt.Sprintf("  Large Title Bars: %s", boolStr(*d.LargeTitleBars)))
	}
	if d.LineCharacters != nil {
		lines = append(lines, fmt.Sprintf("  Line Characters: %s", boolStr(*d.LineCharacters)))
	}
	if d.Shadow != nil {
		lines = append(lines, fmt.Sprintf("  Shadow: %s", boolStr(*d.Shadow)))
	}
	if d.ShadowLevel != nil {
		lines = append(lines, fmt.Sprintf("  Shadow Level: %d", *d.ShadowLevel))
	}
	if d.Scrollbar != nil {
		lines = append(lines, fmt.Sprintf("  Scrollbar: %s", boolStr(*d.Scrollbar)))
	}
	if d.MenuBrackets != nil {
		lines = append(lines, fmt.Sprintf("  Menu Brackets: %s", boolStr(*d.MenuBrackets)))
	}
	if d.BorderColor != nil {
		lines = append(lines, fmt.Sprintf("  Border Color: %d", *d.BorderColor))
	}
	if d.DialogTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Dialog Title: %s", *d.DialogTitleAlign))
	}
	if d.SubmenuTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Submenu Title: %s", *d.SubmenuTitleAlign))
	}
	if d.PanelTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Panel Title: %s", *d.PanelTitleAlign))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Defaults applied by this theme:\n" + strings.Join(lines, "\n")
}

// buildThemeItemHelp returns enriched (itemTitle, itemText) for a theme menu item.
// Used by itemHelpFunc (right-click) and HelpContext (F1).
func (s *DisplayOptionsScreen) buildThemeItemHelp(item displayengine.MenuItem) (itemTitle, itemText string) {
	cv, ok := item.Metadata["config_value"]
	if !ok || cv == "" {
		return "", ""
	}
	tf := s.getThemeFile(cv)

	var parts []string
	desc := tf.Metadata.Description
	if desc == "" {
		// Fallback to what was shown in the list (ThemeMetadata.Description)
		for _, tm := range s.themes {
			if tm.ConfigValue == cv {
				desc = tm.Description
				break
			}
		}
	}
	if desc != "" {
		parts = append(parts, desc)
	}
	if tf.Metadata.Author != "" {
		parts = append(parts, "By: "+tf.Metadata.Author)
	}
	if defaults, derr := theme.FileDefaults(tf); derr == nil {
		if defaultsText := formatThemeDefaults(defaults); defaultsText != "" {
			parts = append(parts, defaultsText)
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	return item.Tag, strings.Join(parts, "\n\n")
}

// HelpContext implements displayengine.HelpContextProvider.
func (s *DisplayOptionsScreen) HelpContext(maxWidth int) displayengine.HelpContext {
	screenName := s.outerMenu.Title()
	pageText := "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options."

	// outerMenu.HelpContext already resolves to whichever leaf is actually
	// focused (via focusedSectionMenu's SubFocusable recursion through
	// ContentColumn/appearanceLayoutRow), so this only needs to layer the
	// theme-specific enrichment on top when that leaf happens to be themeMenu.
	inner := s.outerMenu.HelpContext(maxWidth)
	if s.focusedSettingsMenu() == s.themeMenu {
		items := s.themeMenu.GetItems()
		idx := s.themeMenu.Index()
		if idx >= 0 && idx < len(items) {
			if t, txt := s.buildThemeItemHelp(items[idx]); txt != "" {
				if t != "" {
					inner.ItemTitle = t
				}
				inner.ItemText = txt
			}
		}
	}

	inner.ScreenName = screenName
	inner.PageTitle = "Description"
	inner.PageText = pageText

	return inner
}

func (s *DisplayOptionsScreen) shadowLevelToDesc(l int) string {
	var levels []string
	if s.config.Appearance.Ptr(s.editType).LineCharacters {
		levels = []string{"(Off)", "(░)", "(▒)", "(▓)", "(█)"}
	} else {
		levels = []string{
			"(Off)",
			"({{|Shadow|}}.{{|OptionValue|}})",
			"({{|Shadow|}}:{{|OptionValue|}})",
			"({{|Shadow|}}#{{|OptionValue|}})",
			"({{|Shadow|}}█{{|OptionValue|}})",
		}
	}
	if l < 0 || l >= len(levels) {
		l = 0
	}
	return levels[l]
}

func (s *DisplayOptionsScreen) borderColorToDesc(c int) string {
	modes := map[int]string{1: "(1)", 2: "(2)", 3: "(3D)"}
	return modes[c]
}

func (s *DisplayOptionsScreen) dropdownDesc(val string) string {
	return fmt.Sprintf("{{|OptionValue|}}%s▼{{[-]}}", val)
}

func titleAlignDesc(v string) string {
	if v == "left" {
		return "Left"
	}
	return "Center"
}

func bracketModeDesc(v string) string {
	switch strings.ToLower(v) {
	case "never":
		return "Never"
	case "always":
		return "Always"
	default:
		return "Selected"
	}
}

func tabLayoutDesc(v string) string {
	switch strings.ToLower(v) {
	case "sidebyside":
		return "Side by side"
	case "stacked":
		return "Stacked"
	default:
		return "Maximized"
	}
}

func markdownHyperlinksDesc(v string) string {
	switch strings.ToLower(v) {
	case "off":
		return "Off"
	case "auto":
		return "Auto"
	default:
		return "Inline"
	}
}

// hyperlinksDesc mirrors markdownHyperlinksDesc's shape for ui.hyperlinks --
// same fixed off/inline/auto set, separate function since the two settings
// are independent config fields.
func hyperlinksDesc(v string) string {
	return markdownHyperlinksDesc(v)
}

func (s *DisplayOptionsScreen) panelModeToDesc(v string) string {
	switch strings.ToLower(v) {
	case "none":
		return "None"
	case "log":
		return "Log"
	case "console":
		return "Console"
	case "system":
		return "System Console"
	default:
		return "Default"
	}
}

// radioMenuSelectAction runs the applyFuncs entry for whichever item is
// marked Checked, not the cursor-focused item.
func radioMenuSelectAction(menu *displayengine.MenuModel, applyFuncs []tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		for i, item := range menu.GetItems() {
			if item.Checked && i < len(applyFuncs) && applyFuncs[i] != nil {
				return applyFuncs[i]()
			}
		}
		return displayengine.CloseDialogMsg{}
	}
}

func (s *DisplayOptionsScreen) titleAlignAction(apply func(*config.AppConfig, string), val string) func() tea.Msg {
	return func() tea.Msg {
		return tea.Batch(
			func() tea.Msg {
				return updateDisplayOptionMsg{func(cfg *config.AppConfig) { apply(cfg, val) }}
			},
			tui.CloseDialog(),
		)()
	}
}

func (s *DisplayOptionsScreen) showTitleAlignDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := getter()
		items := []displayengine.MenuItem{
			{Tag: "Left", Help: "Align title to the left", IsRadioButton: true, Selectable: true, Checked: current == "left"},
			{Tag: "Center", Help: "Center the title", IsRadioButton: true, Selectable: true, Checked: current != "left"},
		}
		applyFuncs := []tea.Cmd{s.titleAlignAction(apply, "left"), s.titleAlignAction(apply, "center")}
		menu := displayengine.NewMenuModel(menuName, label, "Select alignment", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor(menuName))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked alignment."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		if current == "left" {
			menu.Select(0)
		} else {
			menu.Select(1)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// showBracketModeDropdown shows the Never/Selected/Always picker for
// ui.checkbox_brackets/ui.radio_brackets. Reuses titleAlignAction, which is
// generic (apply a value, close the dialog) despite its title-align-specific
// name.
func (s *DisplayOptionsScreen) showBracketModeDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := strings.ToLower(getter())
		items := []displayengine.MenuItem{
			{Tag: "Never", Desc: "Only the focused row is bracketed", Help: "Only the focused row is bracketed", IsRadioButton: true, Selectable: true, Checked: current == "never"},
			{Tag: "Selected", Desc: "Bracketed when focused or checked", Help: "Bracketed when focused or checked", IsRadioButton: true, Selectable: true, Checked: current != "never" && current != "always"},
			{Tag: "Always", Desc: "Every row is bracketed", Help: "Every row is bracketed", IsRadioButton: true, Selectable: true, Checked: current == "always"},
		}
		applyFuncs := []tea.Cmd{s.titleAlignAction(apply, "never"), s.titleAlignAction(apply, "selected"), s.titleAlignAction(apply, "always")}
		menu := displayengine.NewMenuModel(menuName, label, "Select mode", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor(menuName))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked mode."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		switch current {
		case "never":
			menu.Select(0)
		case "always":
			menu.Select(2)
		default:
			menu.Select(1)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// showMarkdownHyperlinksDropdown mirrors showBracketModeDropdown's shape for
// ui.markdown_hyperlinks's 3 fixed options.
func (s *DisplayOptionsScreen) showMarkdownHyperlinksDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := strings.ToLower(getter())
		items := []displayengine.MenuItem{
			{Tag: "Off", Desc: "Plain text, no hyperlinks", Help: "Plain text, no hyperlinks", IsRadioButton: true, Selectable: true, Checked: current == "off"},
			{Tag: "Inline", Desc: "Underlined clickable text, URL hidden", Help: "Underlined clickable text, URL hidden", IsRadioButton: true, Selectable: true, Checked: current != "off" && current != "auto"},
			{Tag: "Auto", Desc: "Clickable text plus visible URL", Help: "Clickable text plus visible URL", IsRadioButton: true, Selectable: true, Checked: current == "auto"},
		}
		applyFuncs := []tea.Cmd{s.titleAlignAction(apply, "off"), s.titleAlignAction(apply, "inline"), s.titleAlignAction(apply, "auto")}
		menu := displayengine.NewMenuModel(menuName, label, "Select mode", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor(menuName))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked mode."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		switch current {
		case "off":
			menu.Select(0)
		case "auto":
			menu.Select(2)
		default:
			menu.Select(1)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// showTabLayoutDropdown mirrors showBracketModeDropdown's shape for
// ui.tab_layout's 3 fixed options.
func (s *DisplayOptionsScreen) showTabLayoutDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := strings.ToLower(getter())
		items := []displayengine.MenuItem{
			{Tag: "Maximized", Desc: "Show one tab at a time", Help: "Show one tab at a time", IsRadioButton: true, Selectable: true, Checked: current != "sidebyside" && current != "stacked"},
			{Tag: "Side by side", Desc: "Show tabs side by side", Help: "Show tabs side by side", IsRadioButton: true, Selectable: true, Checked: current == "sidebyside"},
			{Tag: "Stacked", Desc: "Show tabs stacked vertically", Help: "Show tabs stacked vertically", IsRadioButton: true, Selectable: true, Checked: current == "stacked"},
		}
		applyFuncs := []tea.Cmd{s.titleAlignAction(apply, "maximized"), s.titleAlignAction(apply, "sidebyside"), s.titleAlignAction(apply, "stacked")}
		menu := displayengine.NewMenuModel(menuName, label, "Select layout", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor(menuName))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked layout."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		switch current {
		case "sidebyside":
			menu.Select(1)
		case "stacked":
			menu.Select(2)
		default:
			menu.Select(0)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showPanelDropdown() tea.Cmd {
	return func() tea.Msg {
		editType := s.editType
		currentMode := s.config.Appearance.Ptr(editType).Panel

		applyChange := func(mode string) tea.Cmd {
			return func() tea.Msg {
				return tea.Batch(
					func() tea.Msg {
						return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
							cfg.Appearance.Ptr(editType).Panel = mode
						}}
					},
					tui.CloseDialog(),
				)()
			}
		}

		currentLower := strings.ToLower(currentMode)
		var items []displayengine.MenuItem
		var applyFuncs []tea.Cmd

		// None option: always available
		items = append(items, displayengine.MenuItem{
			Tag:           "None",
			Desc:          "Hide the panel entirely",
			Help:          "Removes the panel and stretches content to the bottom of the screen.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       currentLower == "none",
		})
		applyFuncs = append(applyFuncs, func() tea.Msg { return applyChange("none")() })

		// Log option: always available
		items = append(items, displayengine.MenuItem{
			Tag:           "Log",
			Desc:          "Show read-only log viewer",
			Help:          "Displays application logs but hides the command input bar.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       currentLower == "log",
		})
		applyFuncs = append(applyFuncs, func() tea.Msg { return applyChange("log")() })

		// Console (ds2-only): always available for both local and remote —
		// it only accepts ds2 subcommands so it is safe in all session types.
		items = append(items, displayengine.MenuItem{
			Tag:           "Console",
			Desc:          "ds2 commands only",
			Help:          "Accepts ds2 subcommands only. Safe for remote sessions.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       currentLower == "console",
		})
		applyFuncs = append(applyFuncs, func() tea.Msg { return applyChange("console")() })

		// System Console: full shell access.
		// Always show in the dropdown, but require sudo auth if remote.
		systemAction := func() tea.Msg {
			// Warn and require sudo when a remote session enables System
			// Console for a remote connection type.
			if editType != "local" && s.connType != "local" {
				title := "Enable Remote System Console?"
				msg := "System Console grants full interactive shell access to all authenticated " + config.ConnTypeLabel(editType) + " users. Any command, including destructive ones, can be run.\n\nAre you sure you want to proceed?"
				onConfirm := func() tea.Msg {
					// After confirmation, ask for sudo password
					return func() tea.Msg {
						pass, err := tui.PromptText("Sudo Authentication", "Password required to enable System Console:", true)
						if err != nil {
							if err == console.ErrUserAborted {
								return tui.CloseDialog()()
							}
							return tui.ShowMessageDialogMsg{
								Title:   "Authentication Error",
								Message: err.Error(),
								Type:    tui.MessageError,
							}
						}

						if errMsg := verifySudoPassword(pass); errMsg != "" {
							return tui.ShowMessageDialogMsg{
								Title:   "Authentication Failed",
								Message: errMsg,
								Type:    tui.MessageError,
							}
						}

						// Success: Apply the change persistently and close dialog
						return tea.Batch(applyChange("system"), tui.CloseDialog())()
					}()
				}
				confirm := tui.NewConfirmModel(title, msg, false, onConfirm, tui.CloseDialog())
				return displayengine.ShowDialogMsg{Dialog: confirm}
			}
			return applyChange("system")()
		}
		items = append(items, displayengine.MenuItem{
			Tag:           "System Console",
			Desc:          "Full shell access",
			Help:          "Passes commands directly to the OS shell. Use with caution for remote sessions.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       currentLower == "system",
		})
		applyFuncs = append(applyFuncs, systemAction)

		title := config.ConnTypeLabel(editType) + " Panel Mode"
		menu := displayengine.NewMenuModel("panel_dropdown", title, "Choose layout", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor("panel_dropdown"))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked layout."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})

		// Set initial selection — "system" maps to tag "System Console"
		for i, item := range items {
			if strings.ToLower(item.Tag) == currentLower {
				menu.Select(i)
				break
			}
		}

		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showShadowDropdown() tea.Cmd {
	return func() tea.Msg {
		type shadowEntry struct{ label, value string }
		var entries []shadowEntry
		if s.config.Appearance.Ptr(s.editType).LineCharacters {
			entries = []shadowEntry{
				{"Off", ""},
				{"Light", "{{|OptionValue|}}(░){{[-]}}"},
				{"Medium", "{{|OptionValue|}}(▒){{[-]}}"},
				{"Dark", "{{|OptionValue|}}(▓){{[-]}}"},
				{"Solid", "{{|OptionValue|}}(█){{[-]}}"},
			}
		} else {
			entries = []shadowEntry{
				{"Off", ""},
				{"Light", "{{|OptionValue|}}({{|Shadow|}}.{{|OptionValue|}}){{[-]}}"},
				{"Medium", "{{|OptionValue|}}({{|Shadow|}}:{{|OptionValue|}}){{[-]}}"},
				{"Dark", "{{|OptionValue|}}({{|Shadow|}}#{{|OptionValue|}}){{[-]}}"},
				{"Solid", "{{|OptionValue|}}({{|Shadow|}}█{{|OptionValue|}}){{[-]}}"},
			}
		}
		var items []displayengine.MenuItem
		var applyFuncs []tea.Cmd
		for i, e := range entries {
			level := i
			items = append(items, displayengine.MenuItem{
				Tag:           e.label,
				Desc:          e.value,
				Help:          fmt.Sprintf("Set shadow to %s", e.label),
				IsRadioButton: true,
				Selectable:    true,
				Checked:       level == s.config.Appearance.Ptr(s.editType).ShadowLevel,
			})
			applyFuncs = append(applyFuncs, func() tea.Msg {
				return tea.Batch(
					func() tea.Msg {
						return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
							cfg.Appearance.Ptr(s.editType).ShadowLevel = level
						}}
					},
					tui.CloseDialog(),
				)()
			})
		}
		menu := displayengine.NewMenuModel("shadow_dropdown", "Shadow Level", "Select shadow fill pattern", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor("shadow_dropdown"))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked shadow level."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(s.config.Appearance.Ptr(s.editType).ShadowLevel)
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showBorderColorDropdown() tea.Cmd {
	return func() tea.Msg {
		type borderEntry struct {
			mode         int
			label, value string
		}
		entries := []borderEntry{
			{1, "Border 1 (Theme Focus)", "{{|OptionValue|}}(1){{[-]}}"},
			{2, "Border 2 (Theme Accent)", "{{|OptionValue|}}(2){{[-]}}"},
			{3, "Both (3D Effect)", "{{|OptionValue|}}(3D){{[-]}}"},
		}
		var items []displayengine.MenuItem
		var applyFuncs []tea.Cmd
		for _, e := range entries {
			mode := e.mode
			items = append(items, displayengine.MenuItem{
				Tag:           e.label,
				Desc:          e.value,
				Help:          fmt.Sprintf("Set border coloring to %s", e.label),
				IsRadioButton: true,
				Selectable:    true,
				Checked:       mode == s.config.Appearance.Ptr(s.editType).BorderColor,
			})
			applyFuncs = append(applyFuncs, func() tea.Msg {
				return tea.Batch(
					func() tea.Msg {
						return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
							cfg.Appearance.Ptr(s.editType).BorderColor = mode
						}}
					},
					tui.CloseDialog(),
				)()
			})
		}
		menu := displayengine.NewMenuModel("border_dropdown", "Border Coloring", "Select which theme colors highlight borders", items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor("border_dropdown"))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked border coloring."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(s.config.Appearance.Ptr(s.editType).BorderColor - 1)
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) toggleBorders() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).Borders
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).Borders = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLargeButtons() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).LargeButtons
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).LargeButtons = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLargeTitleBars() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).LargeTitleBars
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).LargeTitleBars = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLineChars() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).LineCharacters
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).LineCharacters = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleShadow() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).Shadow
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).Shadow = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleScrollbar() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).Scrollbar
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).Scrollbar = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleMenuBrackets() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).MenuBrackets
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).MenuBrackets = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLineNumberBrackets() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).LineNumberBrackets
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).LineNumberBrackets = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleShowPreview() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).ShowPreview
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).ShowPreview = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleSpinner() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.Appearance.Ptr(s.editType).Spinner
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).Spinner = newState
		}}
	}
}

func (s *DisplayOptionsScreen) promptSpinnerSpeed() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Spinner Speed", "Enter frame speed in milliseconds (50-5000)", false,
			strconv.Itoa(s.config.Appearance.Ptr(s.editType).SpinnerSpeed))
		if err != nil {
			return nil
		}
		ms, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || ms < 50 || ms > 5000 {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Speed",
				Message: "Spinner speed must be between 50 and 5000 milliseconds.",
				Type:    tui.MessageError,
			}
		}
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).SpinnerSpeed = ms
		}}
	}
}

func (s *DisplayOptionsScreen) promptRefreshRate() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Refresh Rate",
			fmt.Sprintf("Enter screen repaint interval in milliseconds (%d-%d)", config.MinRefreshRateMS, config.MaxRefreshRateMS), false,
			strconv.Itoa(s.config.Appearance.Ptr(s.editType).RefreshRate))
		if err != nil {
			return nil
		}
		ms, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || ms < config.MinRefreshRateMS || ms > config.MaxRefreshRateMS {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Refresh Rate",
				Message: fmt.Sprintf("Refresh rate must be between %d and %d milliseconds.", config.MinRefreshRateMS, config.MaxRefreshRateMS),
				Type:    tui.MessageError,
			}
		}
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.Appearance.Ptr(s.editType).RefreshRate = ms
		}}
	}
}

func (s *DisplayOptionsScreen) handleApply() tea.Cmd {
	return func() tea.Msg {
		// Do not apply if any options settings are locked (e.g. panel command running).
		if s.optionsMenu.AnyLocked() {
			return displayOptionsAbortMsg{}
		}
		// 1. Apply Theme (Find the actually checked radio option)
		themeSelected := s.previewTheme
		for _, item := range s.themeMenu.GetItems() {
			if item.Checked {
				if cv, ok := item.Metadata["config_value"]; ok {
					themeSelected = cv
				} else {
					themeSelected = item.Tag
				}
				break
			}
		}

		if _, err := theme.GetThemeFile(themeSelected); err == nil {
			s.currentTheme = themeSelected
			s.config.Appearance.Ptr(s.editType).Theme = themeSelected
		} else {
			s.config.Appearance.Ptr(s.editType).Theme = s.currentTheme
		}

		// 2. Save Config via UpdateAppConfig, applying only this screen's
		// own UI settings onto a freshly-loaded copy, rather than saving
		// s.config (a snapshot from whenever this screen last loaded or
		// saved) wholesale. This screen never touches any other AppConfig
		// field (see the s.config.Appearance assignments above and in
		// display_options_update.go), nor any tint or another connection
		// type's settings, so anything else -- e.g. a tint set
		// from a different session while this screen was open -- would
		// otherwise be clobbered back to its value as of that stale
		// snapshot.
		refreshRateChanged := s.config.Appearance.Ptr(s.editType).RefreshRate != s.baseConfig.Appearance.Ptr(s.editType).RefreshRate
		staged := s.config.Appearance
		// The tint is saved only when edited here, so one set elsewhere
		// meanwhile (e.g. with --tint) isn't overwritten.
		tintEdited := !reflect.DeepEqual(staged.ForConnType(s.editType).AnsiColors, s.baseConfig.Appearance.ForConnType(s.editType).AnsiColors)
		fresh, err := config.UpdateAppConfig(func(c *config.AppConfig) {
			a := c.Appearance.Ptr(s.editType)
			a.CopySettingsFrom(staged.ForConnType(s.editType))
			if tintEdited {
				a.AnsiColors = staged.ForConnType(s.editType).AnsiColors
			}
		})
		if err != nil {
			return tui.ShowMessageDialogMsg{
				Title:   "Save Failed",
				Message: fmt.Sprintf("Could not save appearance settings: %v", err),
				Type:    tui.MessageError,
			}
		}
		s.baseConfig = fresh
		s.config = fresh
		for _, ct := range config.ConnTypes {
			if ct != s.editType {
				s.config.Appearance.Ptr(ct).CopySettingsFrom(staged.ForConnType(ct))
			}
		}

		var previewCmd tea.Cmd
		if s.layoutRow != nil && s.editType == s.connType {
			previewCmd = s.layoutRow.SetPreviewHidden(!s.config.Appearance.Ptr(s.connType).ShowPreview)
		}

		// 3. Trigger synchronized style update
		cmds := []tea.Cmd{func() tea.Msg { return displayengine.ConfigChangedMsg{Config: fresh} }, previewCmd}

		// 4. Refresh rate can only take effect at program construction time
		// (tea.WithFPS has no live-update API). A Local session applying its
		// own refresh rate restarts in place, asking first when that would
		// discard other tabs' unapplied changes. Every other case only
		// affects sessions started later, so it just says so: an SSH or web
		// session reconnecting never needs the whole process restarted.
		if refreshRateChanged && (s.editType != "local" || s.connType != "local") {
			cmds = append(cmds, func() tea.Msg {
				return tui.ShowMessageDialogMsg{
					Title:   "Refresh Rate Saved",
					Message: refreshRateNotice(s.editType),
					Type:    tui.MessageInfo,
				}
			})
		} else if refreshRateChanged {
			unapplied := s.unappliedTabs()
			if len(unapplied) == 0 && tui.IsRestartSafeLocally() {
				tui.RestartForConfigChange(context.Background())
			} else {
				question := "Refresh rate changed. You have unsaved changes — restart now to apply it, or keep editing and it'll apply next session?"
				if len(unapplied) > 0 {
					question = "Refresh rate changed and needs a restart, which would discard the unapplied changes on the " +
						config.ConnTypeLabels(unapplied) + " tab. Restart now, or keep editing and it'll apply next session?"
				}
				resultChan := make(chan bool, 1)
				go func() {
					if <-resultChan {
						tui.RestartForConfigChange(context.Background())
					}
				}()
				cmds = append(cmds, func() tea.Msg {
					return tui.ShowConfirmDialogMsg{
						Title:      "Restart Required",
						Question:   question,
						DefaultYes: false,
						ResultChan: resultChan,
					}
				})
			}
		}

		return tea.Batch(cmds...)()
	}
}

// handleReset discards the shown tab's staged changes, reverting them to
// baseConfig (the settings as of the last Apply,
// or as loaded on screen entry); other tabs keep theirs. Rebuilds the inner
// menus via initMenus so their states reflect the reverted config, then
// re-applies focus since initMenus only resets the bookkeeping fields, not
// the new MenuModels' own focus state.
// tabChanged reports whether connType's staged settings differ from the
// saved ones.
func (s *DisplayOptionsScreen) tabChanged(connType string) bool {
	return !reflect.DeepEqual(s.config.Appearance.ForConnType(connType), s.baseConfig.Appearance.ForConnType(connType))
}

// refreshRateNotice explains when connType's saved refresh rate change
// takes effect.
func refreshRateNotice(connType string) string {
	switch connType {
	case "web":
		return "The Web Server refresh rate applies to web sessions started from now on. To use it in an open browser session, use the gear menu's Apply/Restart, or reload the page."
	case "ssh":
		return "The SSH Server refresh rate applies the next time you connect through the SSH server."
	default:
		return "The Local refresh rate applies the next time DS2 is started in a terminal."
	}
}

// unappliedTabs returns the connection types, other than the shown one,
// with staged changes.
func (s *DisplayOptionsScreen) unappliedTabs() []string {
	var types []string
	for _, ct := range config.ConnTypes {
		if ct != s.editType && s.tabChanged(ct) {
			types = append(types, ct)
		}
	}
	return types
}

func (s *DisplayOptionsScreen) handleReset() tea.Cmd {
	return func() tea.Msg {
		*s.config.Appearance.Ptr(s.editType) = s.baseConfig.Appearance.ForConnType(s.editType)
		s.currentTheme = s.baseConfig.Appearance.ForConnType(s.editType).Theme
		s.previewTheme = s.currentTheme
		s.themeChangedFields = nil
		s.themeDefaults[s.currentTheme], _ = theme.Load(s.currentTheme, "Preview")
		s.initMenus()
		if s.outerMenu != nil {
			s.outerMenu.SetFocused(s.focused)
		}
		return displayengine.ConfigChangedMsg{Config: s.baseConfig}
	}
}

func (s *DisplayOptionsScreen) MenuName() string {
	return "appearance"
}

func (s *DisplayOptionsScreen) IsDestructive() bool {
	return false
}

func (s *DisplayOptionsScreen) FocusTitleBar() {
	if s.outerMenu != nil {
		s.outerMenu.FocusTitleBar()
	}
}

func (s *DisplayOptionsScreen) BlurTitleBar() {
	if s.outerMenu != nil {
		s.outerMenu.BlurTitleBar()
	}
}

func (s *DisplayOptionsScreen) TitleBarFocused() bool {
	return s.outerMenu != nil && s.outerMenu.TitleBarFocused()
}

func (s *DisplayOptionsScreen) Init() tea.Cmd {
	return tea.Batch(s.themeMenu.Init(), s.tintMenu.Init(), s.optionsMenu.Init())
}

func (s *DisplayOptionsScreen) AdvanceSpinners(now time.Time) bool {
	a := s.themeMenu.AdvanceSpinners(now)
	b := s.optionsMenu.AdvanceSpinners(now)
	c := s.tintMenu.AdvanceSpinners(now)
	d := s.outerMenu != nil && s.outerMenu.AdvanceSpinners(now)
	return a || b || c || d
}
