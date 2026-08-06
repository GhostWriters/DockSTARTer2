package theme

import (
	"os"
	"testing"

	semstyle "github.com/GhostWriters/semstyle/lg"
)

func TestEmptyThemeResolvesEveryTag(t *testing.T) {
	data, err := os.ReadFile("../../internal/assets/themes/Empty.ds2theme")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if _, err := parseThemeTOMLData(data, ""); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Tags that are intentionally allowed to resolve empty (deliberate
	// "render unstyled" choices matching what themes themselves do).
	allowedEmpty := map[string]bool{
		"IconInactive":            true,
		"TitleCheckbox":           true,
		"TitleUnfocusedIndicator": true,
		"ItemList":                true, // not fallback-registered by design; caller must define
	}

	names := []string{
		"Screen", "Dialog", "StatusBar", "Tag", "Item", "Title",
		"ButtonInactive", "ButtonActive", "IconInactive", "IconFocused", "IconPressed",
		"Highlight", "MarkerInvalid", "ItemFocused", "TagFocused", "TagKey", "TagKeyFocused",
		"TagBrackets", "TagSpinner", "ItemList", "ItemListUserDefined",
		"ButtonKeyActive", "ButtonKeyInactive", "ButtonSpinner", "LargeButtonSpinner",
		"CommandLine", "Subtitle", "ProgramBox", "Shadow", "OptionValue", "OptionValueFocused",
		"LargeTitle", "LargeTitleArea", "StatusFields", "StatusFlagsBrackets", "StatusUpdate",
		"StatusVersionFocused", "Heading", "HeadingTag", "HeadingValue", "HeadingAppDescription",
		"MarkerLocked", "MarkerAdded", "MarkerDeleted", "MarkerModified", "ModifiedText",
		"EnvInvalid", "EnvDuplicate", "EnvBuiltin", "EnvReadOnly", "EnvPendingDelete",
		"FailingCommand", "Yes", "URL", "LineComment", "LineNumber", "LineNumberBrackets",
		"LineNumberFocused", "LineNumberModified", "LineNumberModifiedFocused",
		"Scrollbar", "ScrollbarThumb", "ScrollbarArrows", "KeyCap",
		"ProgressWaiting", "ProgressInProgress", "ProgressCompleted", "TextCursor",
		"TitleCheckbox", "TitleCheckboxFocused", "TitleFocusIndicator", "TitleUnfocusedIndicator",
		"PanelTitle", "PanelTitleChangedIndicator",
		// A sampling of deep-chain tags to prove multi-hop resolution works.
		"LargeTitleAreaHelp", "LargeIconHelpFocused", "TitleWarnFocused",
		"LargeTitleWarnFocused", "CheckboxOnFocused", "RadioOnFocused",
		"ButtonLockedMarker", "LargeButtonLockedMarker", "HelpTag", "HelpItem",
		// Border/Border2: not reachable via a literal tag reference anywhere
		// (looked up through a variable tag name in ResolveThemeOverrides),
		// so this session's earlier code-usage sweep missed them entirely --
		// caught only by actually rendering the Empty theme, which broke the
		// top border because Border's raw code also feeds the parsed
		// border-drawing flags, not just its color.
		"Border", "Border2",
	}

	for _, name := range names {
		got := semstyle.GetRawTagCode(name)
		if got == "" && !allowedEmpty[name] {
			t.Errorf("%s resolved empty with the Empty theme", name)
		}
	}
}
