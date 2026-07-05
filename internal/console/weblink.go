package console

// FormatLink returns raw (unresolved), unquoted semstyle tag markup for a
// label wrapped in a hyperlink tag pointing to url -- e.g. a version number
// linking to its GitHub release page, or a service name linking to its docs
// page. Unlike FormatFilePath/FormatFileName, this has no eligibility check:
// an http(s) URL is equally reachable from a local terminal, an SSH client,
// or a browser, so there's no "different machine" concern the way there is
// for file:// links. An empty url renders label as plain styled text with
// no link at all.
func FormatLink(tag, label, url string) string {
	if url == "" {
		return "{{|" + tag + "|}}" + label + "{{[-]}}"
	}
	return "{{|" + tag + "::::" + url + "|}}" + label + "{{[-]}}"
}
