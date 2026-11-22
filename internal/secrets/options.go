package secrets

import "github.com/frostyard/frostnotes/internal/config"

// Options say what Open does when FROSTNOTES_SECRETS is unset and no Secret Service answers.
type Options struct {
	// NoFallback makes Open keep no plain file: its store fails every Get, Set and Delete with an error that wraps ErrNoKeyring.
	NoFallback bool
	// Hint ends the message of ErrNoKeyring errors: how the user keeps tokens in a file instead.
	Hint string
}

// HostOptions allow a plain file when no Secret Service answers on the host.
func HostOptions() Options {
	return Options{Hint: "set FROSTNOTES_SECRETS=file to keep tokens in a file"}
}

// FlatpakOptions keep no plain file unless the user explicitly opts in.
func FlatpakOptions() Options {
	return Options{
		NoFallback: true,
		Hint:       "run `flatpak override --user --env=FROSTNOTES_SECRETS=file " + config.AppID + "` to keep tokens in a plain file instead",
	}
}

// OptionsFor select the secret-store policy for notesd's environment.
func OptionsFor(inFlatpak bool) Options {
	if inFlatpak {
		return FlatpakOptions()
	}
	return HostOptions()
}
