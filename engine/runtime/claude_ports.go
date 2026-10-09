package runtime

// HookInstaller is how the Claude adapter reaches the hooks in Claude Code's settings.json. The
// adapter owns the port; the settings file adapter (engine/settings/settingsfile) answers it, and
// the composition root wires the two. A test of the adapter hands it a fake, or the real installer
// over a temporary directory, and neither can touch the settings of the person who runs it.
//
// Every method takes the settings file and the binary the hook entries run, because the adapter
// resolves both from the Claude root it was built over; the installer keeps no state.
type HookInstaller interface {
	// Install adds the hook entries the overlay owns for the binary at hookCommand to the settings
	// file at path, creating the file if it is absent and keeping the one it replaces. A file that
	// is not valid JSON is left as it is and the error says so.
	Install(path, hookCommand string) error
	// Uninstall removes exactly those entries. A file that is absent has none, and that is not an
	// error.
	Uninstall(path, hookCommand string) error
	// Inspect says what the settings file holds. found is false when there is no settings file
	// (or it holds nothing); owned is true when every hook family the overlay installs is in place
	// for the binary at hookCommand. An error means the file could not be read or parsed, and is
	// the one the system or the JSON decoder reported.
	Inspect(path, hookCommand string) (found, owned bool, err error)
}
