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

// --- Tag delimiter values ---
var (
	SemanticPrefix = semstyle.SemanticPrefix
	SemanticSuffix = semstyle.SemanticSuffix
	DirectPrefix   = semstyle.DirectPrefix
	DirectSuffix   = semstyle.DirectSuffix
)

// --- Core rendering ---
var (
	ToANSI  = semstyle.ToANSI
	ToTags  = semstyle.ToTags
	ToPlain = semstyle.ToPlain
	Sprintf = semstyle.Sprintf
)

// --- Color conversion ---
var (
	ToColor      = semstyle.ToColor
	ToColorStr   = semstyle.ToColorStr
	GetHexForColor    = semstyle.GetHexForColor
	ResolveTcellColor = semstyle.ResolveTcellColor
)

// --- Tag stripping ---
var (
	StripTags  = semstyle.StripTags
	StripANSI  = semstyle.StripANSI
)

// --- Tag utilities ---
var (
	StripDelimiters    = semstyle.StripDelimiters
	SetDelimiters      = semstyle.SetDelimiters
	GetDelimitedRegex  = semstyle.GetDelimitedRegex
	GetDirectRegex     = semstyle.GetDirectRegex
	WrapDirect         = semstyle.WrapDirect
	WrapSemantic       = semstyle.WrapSemantic
	ExpandTagsWithMap  = semstyle.ExpandTagsWithMap
	GetColorDefinition = semstyle.GetColorDefinition
	GetRawTagCode      = semstyle.GetRawTagCode
)

// --- Tag registration ---
var (
	BuildColorMap          = semstyle.BuildColorMap
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
)

// --- Constants (ANSI codes) ---
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
