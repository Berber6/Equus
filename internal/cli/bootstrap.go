package cli

// init ensures the Equus home directory exists before commands run.
// Best-effort: commands surface real errors through loadConfig.
func init() {
	_ = EnsureHome()
}
