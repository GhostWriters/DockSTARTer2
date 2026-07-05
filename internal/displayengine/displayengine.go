// Package displayengine re-exports the public API of the active low-level
// rendering engine (currently internal/displayengine/classic) so call sites
// in internal/tui can depend on a stable import path independent of which
// concrete engine implementation is in use.
package displayengine

import (
	"DockSTARTer2/internal/displayengine/classic"

	tea "charm.land/bubbletea/v2"
)

// DialogWithBackdrop is generic, so it is aliased with its own type
// parameter list rather than being grouped with the other type aliases.
type DialogWithBackdrop[T DialogModel] = classic.DialogWithBackdrop[T]

// NewDialogWithBackdrop is generic, so it is re-exported as a thin wrapper
// function rather than a var (Go does not support generic vars/funcs
// assigned from another generic func while preserving inference).
func NewDialogWithBackdrop[T DialogModel](dialog T, helpText string) DialogWithBackdrop[T] {
	return classic.NewDialogWithBackdrop(dialog, helpText)
}

// SetConfirmExitFallback overrides classic's Esc/exit-button fallback tea.Cmd.
// A plain `displayengine.ConfirmExitFallback = classic.ConfirmExitFallback`
// var-copy would only ever reassign displayengine's own copy of the func
// value, never the package-level var inside classic that MenuModel.Update
// actually reads -- so this must be a real setter, not a var alias.
func SetConfirmExitFallback(fn func() tea.Cmd) { classic.ConfirmExitFallback = fn }

// SetHeaderUpdateHook overrides classic's header click/wheel reaction hook.
// See SetConfirmExitFallback's doc comment for why this can't be a plain var re-export.
func SetHeaderUpdateHook(fn func(h *HeaderModel, msg tea.Msg) (tea.Model, tea.Cmd)) {
	classic.HeaderUpdateHook = fn
}

// SetPromptTextHook overrides classic's blocking text-prompt hook.
// See SetConfirmExitFallback's doc comment for why this can't be a plain var re-export.
func SetPromptTextHook(fn func(title, question string, sensitive bool) (string, error)) {
	classic.PromptTextHook = fn
}

// Constants
const (
	HoverButton            = classic.HoverButton
	LargeTitleBarOverhead  = classic.LargeTitleBarOverhead
	MinDialogHeight        = classic.MinDialogHeight
	ScrollbarGutterWidth   = classic.ScrollbarGutterWidth
	DialogBodyPadH         = classic.DialogBodyPadH
	ZDialog                = classic.ZDialog
	DialogTypeInfo         = classic.DialogTypeInfo
	DialogTypeSuccess      = classic.DialogTypeSuccess
	DialogTypeWarning      = classic.DialogTypeWarning
	DialogTypeError        = classic.DialogTypeError
	DialogTypeConfirm      = classic.DialogTypeConfirm
	BorderStyleRounded     = classic.BorderStyleRounded
	DialogShadowHeight     = classic.DialogShadowHeight
	DialogShadowWidth      = classic.DialogShadowWidth
	FocusList              = classic.FocusList
	HeaderFocusNone        = classic.HeaderFocusNone
	HeaderFocusApp         = classic.HeaderFocusApp
	HeaderFocusTmpl        = classic.HeaderFocusTmpl
	HeaderFocusFlags       = classic.HeaderFocusFlags
	HeaderFocusWebDisplay  = classic.HeaderFocusWebDisplay
	IDConsoleInput         = classic.IDConsoleInput
	IDAppVersion           = classic.IDAppVersion
	IDTmplVersion          = classic.IDTmplVersion
	IDPanel                = classic.IDPanel
	IDPanelToggle          = classic.IDPanelToggle
	IDPanelResize          = classic.IDPanelResize
	IDPanelViewport        = classic.IDPanelViewport
	IDStatusBar            = classic.IDStatusBar
	IDThemePanel           = classic.IDThemePanel
	IDOptionsPanel         = classic.IDOptionsPanel
	IDButtonPanel          = classic.IDButtonPanel
	IDListPanel            = classic.IDListPanel
	IDApplyButton          = classic.IDApplyButton
	IDBackButton           = classic.IDBackButton
	IDExitButton           = classic.IDExitButton
	IDHeaderFlags          = classic.IDHeaderFlags
	IDHeaderWebDisplay     = classic.IDHeaderWebDisplay
	IDInsOvr               = classic.IDInsOvr
	IDPanelResizeUp        = classic.IDPanelResizeUp
	IDPanelResizeDn        = classic.IDPanelResizeDn
	OverlayCenter          = classic.OverlayCenter
	ZScreen                = classic.ZScreen
	ZPanel                 = classic.ZPanel
	ZModalBaseOffset       = classic.ZModalBaseOffset
	ZModalStackStep        = classic.ZModalStackStep
	DialogMaximized        = classic.DialogMaximized
	DialogAbsoluteCentered = classic.DialogAbsoluteCentered
	ResizeZoneID           = classic.ResizeZoneID
	PanelWidgetUp          = classic.PanelWidgetUp
	PanelWidgetDn          = classic.PanelWidgetDn
	FocusBtn               = classic.FocusBtn
	ColAdd                 = classic.ColAdd
	ColEnable              = classic.ColEnable
	ColExpand              = classic.ColExpand
	ColName                = classic.ColName
	IDTitleWidgetClose     = classic.IDTitleWidgetClose
	IDTitleWidgetHelp      = classic.IDTitleWidgetHelp
	IDTitleWidgetRefresh   = classic.IDTitleWidgetRefresh
	IDSaveButton           = classic.IDSaveButton
	DialogButtonHeight     = classic.DialogButtonHeight
)

// Types
type (
	BackdropModel              = classic.BackdropModel
	BaseDialogModel            = classic.BaseDialogModel
	BorderPair                 = classic.BorderPair
	BorderStyle                = classic.BorderStyle
	ButtonDef                  = classic.ButtonDef
	ButtonLayout               = classic.ButtonLayout
	ButtonRow                  = classic.ButtonRow
	ButtonSpec                 = classic.ButtonSpec
	CheckboxColumn             = classic.CheckboxColumn
	CloseDialogMsg             = classic.CloseDialogMsg
	ConfigChangedMsg           = classic.ConfigChangedMsg
	ConsoleLockMsg             = classic.ConsoleLockMsg
	Content                    = classic.Content
	ContentRow                 = classic.ContentRow
	ContextMenuItem            = classic.ContextMenuItem
	ContextMenuModel           = classic.ContextMenuModel
	DialogLayout               = classic.DialogLayout
	DialogMode                 = classic.DialogMode
	DialogModel                = classic.DialogModel
	DialogPosition             = classic.DialogPosition
	DialogType                 = classic.DialogType
	DragDoneMsg                = classic.DragDoneMsg
	DynamicHelpProvider        = classic.DynamicHelpProvider
	FocusItem                  = classic.FocusItem
	HeaderFocus                = classic.HeaderFocus
	HeaderModel                = classic.HeaderModel
	HelpContext                = classic.HelpContext
	HelpContextProvider        = classic.HelpContextProvider
	HelplineModel              = classic.HelplineModel
	HitRegion                  = classic.HitRegion
	HitRegionProvider          = classic.HitRegionProvider
	HitRegions                 = classic.HitRegions
	KeyMap                     = classic.KeyMap
	LayerHitMsg                = classic.LayerHitMsg
	LayerSpec                  = classic.LayerSpec
	LayerWheelMsg              = classic.LayerWheelMsg
	Layout                     = classic.Layout
	LockStateChangedMsg        = classic.LockStateChangedMsg
	MenuItem                   = classic.MenuItem
	MenuModel                  = classic.MenuModel
	OverlayPosition            = classic.OverlayPosition
	PanelCommandLockChangedMsg = classic.PanelCommandLockChangedMsg
	PanelModel                 = classic.PanelModel
	RefreshHeaderMsg           = classic.RefreshHeaderMsg
	ScrollDoneMsg              = classic.ScrollDoneMsg
	Scrollbar                  = classic.Scrollbar
	ScrollbarDragState         = classic.ScrollbarDragState
	ScrollbarInfo              = classic.ScrollbarInfo
	ShowDialogMsg              = classic.ShowDialogMsg
	Sizer                      = classic.Sizer
	StyleContext               = classic.StyleContext
	Styles                     = classic.Styles
	TitleBarFocus              = classic.TitleBarFocus
	TitleBarFocusable          = classic.TitleBarFocusable
	TitleBarRefreshMsg         = classic.TitleBarRefreshMsg
	TitleBarState              = classic.TitleBarState
	TitleBarWidgetHelper       = classic.TitleBarWidgetHelper
	TitleSpinner               = classic.TitleSpinner
	ToggleFocusedMsg           = classic.ToggleFocusedMsg
	TriggerHelpMsg             = classic.TriggerHelpMsg
	WidgetDef                  = classic.WidgetDef
	ReplaceOutputMsg           = classic.ReplaceOutputMsg
	TogglePanelMsg             = classic.TogglePanelMsg
	WidgetClearPressMsg        = classic.WidgetClearPressMsg
	MenuDeferredActionMsg      = classic.MenuDeferredActionMsg
	PanelLineMsg               = classic.PanelLineMsg
	ConsoleLinesMsg            = classic.ConsoleLinesMsg
	ConsoleDoneMsg             = classic.ConsoleDoneMsg
)

// Vars
var (
	AsciiBorder             = classic.AsciiBorder
	WidgetRefresh           = classic.WidgetRefresh
	WidgetHelp              = classic.WidgetHelp
	WidgetClose             = classic.WidgetClose
	Keys                    = classic.Keys
	RoundedAsciiBorder      = classic.RoundedAsciiBorder
	RoundedThickAsciiBorder = classic.RoundedThickAsciiBorder
	SlantedAsciiBorder      = classic.SlantedAsciiBorder
	SlantedBorder           = classic.SlantedBorder
	SlantedThickAsciiBorder = classic.SlantedThickAsciiBorder
	SlantedThickBorder      = classic.SlantedThickBorder
	ThickRoundedBorder      = classic.ThickRoundedBorder
)

// Funcs
var (
	AddHalo                        = classic.AddHalo
	AddHaloCtx                     = classic.AddHaloCtx
	AddPatternHalo                 = classic.AddPatternHalo
	AddShadow                      = classic.AddShadow
	AddShadowCtx                   = classic.AddShadowCtx
	AppendContextMenuTail          = classic.AppendContextMenuTail
	Apply3DBorder                  = classic.Apply3DBorder
	Apply3DBorderCtx               = classic.Apply3DBorderCtx
	ApplyInnerBorder               = classic.ApplyInnerBorder
	ApplyInnerBorderCtx            = classic.ApplyInnerBorderCtx
	ApplyRoundedBorder             = classic.ApplyRoundedBorder
	ApplyRoundedBorderCtx          = classic.ApplyRoundedBorderCtx
	ApplyScrollbar                 = classic.ApplyScrollbar
	ApplyScrollbarColumn           = classic.ApplyScrollbarColumn
	ApplyScrollbarColumnTracked    = classic.ApplyScrollbarColumnTracked
	ApplySlantedBorder             = classic.ApplySlantedBorder
	ApplySlantedBorderCtx          = classic.ApplySlantedBorderCtx
	ApplyStraightBorder            = classic.ApplyStraightBorder
	ApplyStraightBorderCtx         = classic.ApplyStraightBorderCtx
	ApplyThickBorder               = classic.ApplyThickBorder
	ApplyThickBorderCtx            = classic.ApplyThickBorderCtx
	BuildAEBottomBorder            = classic.BuildAEBottomBorder
	BuildAETopBorder               = classic.BuildAETopBorder
	BuildDualLabelBottomBorderCtx  = classic.BuildDualLabelBottomBorderCtx
	BuildInactiveLargeTitleWidgets = classic.BuildInactiveLargeTitleWidgets
	BuildInactiveTitleWidgets      = classic.BuildInactiveTitleWidgets
	BuildInactiveTitleWidgetsFor   = classic.BuildInactiveTitleWidgetsFor
	BuildLabeledBottomBorderCtx    = classic.BuildLabeledBottomBorderCtx
	BuildPlainBottomBorder         = classic.BuildPlainBottomBorder
	BuildScrollPercentBottomBorder = classic.BuildScrollPercentBottomBorder
	ButtonIDMatches                = classic.ButtonIDMatches
	ButtonRowHeight                = classic.ButtonRowHeight
	CenterText                     = classic.CenterText
	CheckButtonHotkeys             = classic.CheckButtonHotkeys
	ClearSemanticCache             = classic.ClearSemanticCache
	ClearSemanticCachePrefix       = classic.ClearSemanticCachePrefix
	CurrentConfig                  = classic.CurrentConfig
	CodeToStyle                    = classic.CodeToStyle
	ComputeButtonLayout            = classic.ComputeButtonLayout
	ComputeScrollbarInfo           = classic.ComputeScrollbarInfo
	DecideLargeTitleBar            = classic.DecideLargeTitleBar
	DefaultLayout                  = classic.DefaultLayout
	DragDoneCmd                    = classic.DragDoneCmd
	EffectivePanelMode             = classic.EffectivePanelMode
	EnforceDialogLayout            = classic.EnforceDialogLayout
	GetActiveContentStartY         = classic.GetActiveContentStartY
	GetActiveDialogOffset          = classic.GetActiveDialogOffset
	GetActiveScreenSize            = classic.GetActiveScreenSize
	GetActiveContext               = classic.GetActiveContext
	GetAvailableDialogSize         = classic.GetAvailableDialogSize
	GetBlockBorders                = classic.GetBlockBorders
	GetButtonHitRegions            = classic.GetButtonHitRegions
	GetButtonHitRegionsExplicit    = classic.GetButtonHitRegionsExplicit
	GetInitialStyle                = classic.GetInitialStyle
	GetLayout                      = classic.GetLayout
	GetMenuItemID                  = classic.GetMenuItemID
	GetPlainText                   = classic.GetPlainText
	GetPositionCenter              = classic.GetPositionCenter
	GetPositionTopLeft             = classic.GetPositionTopLeft
	GetShadowBoxCtx                = classic.GetShadowBoxCtx
	GetSolidBoxCtx                 = classic.GetSolidBoxCtx
	GetStyles                      = classic.GetStyles
	HandleScrollbarLayerHit        = classic.HandleScrollbarLayerHit
	HelpContextWidth               = classic.HelpContextWidth
	InitStyles                     = classic.InitStyles
	InjectBorderFlags              = classic.InjectBorderFlags
	IsScrollbarEnabled             = classic.IsScrollbarEnabled
	IsTitleWidgetID                = classic.IsTitleWidgetID
	MaintainBackground             = classic.MaintainBackground
	MaxLineWidth                   = classic.MaxLineWidth
	MinWidthForWidgets             = classic.MinWidthForWidgets
	MultiOverlay                   = classic.MultiOverlay
	MultiOverlayWithBounds         = classic.MultiOverlayWithBounds
	NewBackdropModel               = classic.NewBackdropModel
	NewButtonRow                   = classic.NewButtonRow
	NewContentRow                  = classic.NewContentRow
	NewContextMenuModel            = classic.NewContextMenuModel
	NewHeaderModel                 = classic.NewHeaderModel
	NewHelplineModel               = classic.NewHelplineModel
	NewMenuModel                   = classic.NewMenuModel
	NewNumberSinputSection         = classic.NewNumberSinputSection
	NewPanelModel                  = classic.NewPanelModel
	NewPasswordSinputSection       = classic.NewPasswordSinputSection
	NewPlainTextSection            = classic.NewPlainTextSection
	NewSinputSection               = classic.NewSinputSection
	OutputContentWidth             = classic.OutputContentWidth
	Overlay                        = classic.Overlay
	PadRight                       = classic.PadRight
	ParseColor                     = classic.ParseColor
	ParseMenuItemIndex             = classic.ParseMenuItemIndex
	Render3DBorder                 = classic.Render3DBorder
	Render3DBorderCtx              = classic.Render3DBorderCtx
	RenderBorderedBoxCtx           = classic.RenderBorderedBoxCtx
	RenderButton                   = classic.RenderButton
	RenderButtonRow                = classic.RenderButtonRow
	RenderCenteredButtons          = classic.RenderCenteredButtons
	RenderCenteredButtonsCtx       = classic.RenderCenteredButtonsCtx
	RenderCenteredButtonsExplicit  = classic.RenderCenteredButtonsExplicit
	RenderConsoleText              = classic.RenderConsoleText
	RenderConsoleTextCtx           = classic.RenderConsoleTextCtx
	RenderDialog                   = classic.RenderDialog
	RenderDialogBox                = classic.RenderDialogBox
	RenderDialogBoxCtx             = classic.RenderDialogBoxCtx
	RenderDialogCtx                = classic.RenderDialogCtx
	RenderDialogWithType           = classic.RenderDialogWithType
	RenderDialogWithTypeAndWidgets = classic.RenderDialogWithTypeAndWidgets
	RenderDialogWithTypeCtx        = classic.RenderDialogWithTypeCtx
	RenderHotkeyLabel              = classic.RenderHotkeyLabel
	RenderHotkeyLabelCtx           = classic.RenderHotkeyLabelCtx
	RenderMenuGutter               = classic.RenderMenuGutter
	RenderThemeText                = classic.RenderThemeText
	RenderThemeTextCtx             = classic.RenderThemeTextCtx
	RenderTitleSegmentCtx          = classic.RenderTitleSegmentCtx
	RenderTopBorderBoxCtx          = classic.RenderTopBorderBoxCtx
	RenderUniformBlockDialog       = classic.RenderUniformBlockDialog
	RenderUniformBlockDialogCtx    = classic.RenderUniformBlockDialogCtx
	RenderWithBackdrop             = classic.RenderWithBackdrop
	ScanForHyperlinks              = classic.ScanForHyperlinks
	StripHyperlinks                = classic.StripHyperlinks
	HyperlinkPath                  = classic.HyperlinkPath
	HyperlinkText                  = classic.HyperlinkText
	ScrollbarHitRegions            = classic.ScrollbarHitRegions
	SemanticRawStyle               = classic.SemanticRawStyle
	SemanticStyle                  = classic.SemanticStyle
	SetActiveContentStartY         = classic.SetActiveContentStartY
	SetActiveDialogOffset          = classic.SetActiveDialogOffset
	SetActiveOutputWidth           = classic.SetActiveOutputWidth
	SetActiveScreenSize            = classic.SetActiveScreenSize
	ShowInputContextMenu           = classic.ShowInputContextMenu
	ShowInputContextMenuWithTitle  = classic.ShowInputContextMenuWithTitle
	SinputSectionInit              = classic.SinputSectionInit
	SplitWidth                     = classic.SplitWidth
	TextCursorColor                = classic.TextCursorColor
	TitleBarWidgetRegions          = classic.TitleBarWidgetRegions
	TitleBarWidgetY                = classic.TitleBarWidgetY
	ToStyle                        = classic.ToStyle
	TruncateLeft                   = classic.TruncateLeft
	TruncateRight                  = classic.TruncateRight
	WidthOfTitleSegment            = classic.WidthOfTitleSegment
	WidthWithoutZones              = classic.WidthWithoutZones
)
