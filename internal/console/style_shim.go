package console

// This file re-exports the semantic styling engine (internal/semstyle) under the
// console package so existing console.* call sites keep working unchanged. The styling
// implementation now lives in semstyle; console retains only the app/TTY plumbing.
//
// This shim is intentional and temporary: a later pass can migrate call sites to import
// semstyle directly and delete this file. Until then, console.X == semstyle.X.

import (
	"DockSTARTer2/internal/semstyle"
)

// --- Tag delimiter values (set once at init; read-only consumers) ---
// The engine owns the authoritative values; SetDelimiters updates them there. These are
// convenience re-exports for the common read-only case.
var (
	SemanticPrefix = semstyle.SemanticPrefix
	SemanticSuffix = semstyle.SemanticSuffix
	DirectPrefix   = semstyle.DirectPrefix
	DirectSuffix   = semstyle.DirectSuffix
)

// --- Functions ---
var (
	ToConsoleANSI         = semstyle.ToConsoleANSI
	ToThemeANSI           = semstyle.ToThemeANSI
	ToThemeANSIWithPrefix = semstyle.ToThemeANSIWithPrefix
	ForTUI                = semstyle.ForTUI
	Sprintf               = semstyle.Sprintf
	Strip                 = semstyle.Strip
	StripANSI             = semstyle.StripANSI
	StripDelimiters       = semstyle.StripDelimiters
	StripSemanticTags     = semstyle.StripSemanticTags
	ExpandConsoleTags     = semstyle.ExpandConsoleTags
	ExpandThemeTags       = semstyle.ExpandThemeTags
	ExpandSemanticTags    = semstyle.ExpandSemanticTags
	ExpandTagsWithMap     = semstyle.ExpandTagsWithMap
	WrapDirect            = semstyle.WrapDirect
	WrapSemantic          = semstyle.WrapSemantic
	SetDelimiters         = semstyle.SetDelimiters
	GetDelimitedRegex     = semstyle.GetDelimitedRegex
	GetDirectRegex        = semstyle.GetDirectRegex

	ParseColor         = semstyle.ParseColor
	GetColorStr        = semstyle.GetColorStr
	GetHexForColor     = semstyle.GetHexForColor
	ResolveTcellColor  = semstyle.ResolveTcellColor
	GetColorDefinition = semstyle.GetColorDefinition
	GetRawTagCode      = semstyle.GetRawTagCode

	BuildColorMap          = semstyle.BuildColorMap
	RegisterBaseTags       = semstyle.RegisterBaseTags
	RegisterHyperlinkTag   = semstyle.RegisterHyperlinkTag
	RegisterColor          = semstyle.RegisterColor
	UnregisterColor        = semstyle.UnregisterColor
	UnregisterPrefix       = semstyle.UnregisterPrefix
	ResetCustomColors      = semstyle.ResetCustomColors
	RegisterConsoleTag     = semstyle.RegisterConsoleTag
	RegisterConsoleTagRaw  = semstyle.RegisterConsoleTagRaw
	RegisterThemeTag       = semstyle.RegisterThemeTag
	RegisterThemeTagRaw    = semstyle.RegisterThemeTagRaw
	RegisterSemanticTag    = semstyle.RegisterSemanticTag
	RegisterSemanticTagRaw = semstyle.RegisterSemanticTagRaw
	ClearThemeMap          = semstyle.ClearThemeMap
	Translate              = semstyle.Translate
	TranslateToTagged      = semstyle.TranslateToTagged
	ToCviewTag             = semstyle.ToCviewTag
)

// --- Types ---
type AppColors = semstyle.AppColors

// --- Constants (ANSI codes + delimiters) ---
const (
	CodeReset            = semstyle.CodeReset
	CodeHardReset        = semstyle.CodeHardReset
	CodeBold             = semstyle.CodeBold
	CodeBoldOff          = semstyle.CodeBoldOff
	CodeDim              = semstyle.CodeDim
	CodeDimOff           = semstyle.CodeDimOff
	CodeUnderline        = semstyle.CodeUnderline
	CodeUnderlineOff     = semstyle.CodeUnderlineOff
	CodeBlink            = semstyle.CodeBlink
	CodeBlinkOff         = semstyle.CodeBlinkOff
	CodeReverse          = semstyle.CodeReverse
	CodeReverseOff       = semstyle.CodeReverseOff
	CodeItalic           = semstyle.CodeItalic
	CodeItalicOff        = semstyle.CodeItalicOff
	CodeStrikethrough    = semstyle.CodeStrikethrough
	CodeStrikethroughOff = semstyle.CodeStrikethroughOff
	CodeFGReset          = semstyle.CodeFGReset
	CodeBGReset          = semstyle.CodeBGReset
	CodeHardFGReset      = semstyle.CodeHardFGReset
	CodeHardBGReset      = semstyle.CodeHardBGReset

	CodeBlack   = semstyle.CodeBlack
	CodeRed     = semstyle.CodeRed
	CodeGreen   = semstyle.CodeGreen
	CodeYellow  = semstyle.CodeYellow
	CodeBlue    = semstyle.CodeBlue
	CodeMagenta = semstyle.CodeMagenta
	CodeCyan    = semstyle.CodeCyan
	CodeWhite   = semstyle.CodeWhite

	CodeBrightBlack   = semstyle.CodeBrightBlack
	CodeBrightRed     = semstyle.CodeBrightRed
	CodeBrightGreen   = semstyle.CodeBrightGreen
	CodeBrightYellow  = semstyle.CodeBrightYellow
	CodeBrightBlue    = semstyle.CodeBrightBlue
	CodeBrightMagenta = semstyle.CodeBrightMagenta
	CodeBrightCyan    = semstyle.CodeBrightCyan
	CodeBrightWhite   = semstyle.CodeBrightWhite

	CodeBlackBg   = semstyle.CodeBlackBg
	CodeRedBg     = semstyle.CodeRedBg
	CodeGreenBg   = semstyle.CodeGreenBg
	CodeYellowBg  = semstyle.CodeYellowBg
	CodeBlueBg    = semstyle.CodeBlueBg
	CodeMagentaBg = semstyle.CodeMagentaBg
	CodeCyanBg    = semstyle.CodeCyanBg
	CodeWhiteBg   = semstyle.CodeWhiteBg

	CodeBrightBlackBg   = semstyle.CodeBrightBlackBg
	CodeBrightRedBg     = semstyle.CodeBrightRedBg
	CodeBrightGreenBg   = semstyle.CodeBrightGreenBg
	CodeBrightYellowBg  = semstyle.CodeBrightYellowBg
	CodeBrightBlueBg    = semstyle.CodeBrightBlueBg
	CodeBrightMagentaBg = semstyle.CodeBrightMagentaBg
	CodeBrightCyanBg    = semstyle.CodeBrightCyanBg
	CodeBrightWhiteBg   = semstyle.CodeBrightWhiteBg
)
