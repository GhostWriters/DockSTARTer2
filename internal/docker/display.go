package docker

import "DockSTARTer2/internal/dockerlayout"

// Re-export dockerlayout symbols so callers in this package use a single import.
const (
	GlobalIndentW        = dockerlayout.GlobalIndentW
	IconW                = dockerlayout.IconW
	SpaceW               = dockerlayout.SpaceW
	SectionStatusTextW   = dockerlayout.SectionStatusTextW
	SectionStatusGutterW = dockerlayout.SectionStatusGutterW
	SectionStatusW       = dockerlayout.SectionStatusW
	SectionChildIndentW  = dockerlayout.SectionChildIndentW
	ImageLabelTextW      = dockerlayout.ImageLabelTextW
	TimerGutterW         = dockerlayout.TimerGutterW
	SectionHeaderIndent  = dockerlayout.SectionHeaderIndent
	ImageLabelW          = dockerlayout.ImageLabelW
	LayerPrefixW         = dockerlayout.LayerPrefixW
	LayerStatusW         = dockerlayout.LayerStatusW
)

var (
	GlobalIndent       = dockerlayout.GlobalIndent
	SectionChildIndent = dockerlayout.SectionChildIndent
	LayerPrefix        = dockerlayout.LayerPrefix
)

var (
	AbbreviateStatus = dockerlayout.AbbreviateStatus
	Plural           = dockerlayout.Plural
	StyleImageRef    = dockerlayout.StyleImageRef
)
