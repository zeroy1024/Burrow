package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// Plain go run and MyGo's dev bundle share a development namespace. Release
// bundles use Burrow; tests use temporary stores and never call Run.
func configureDataDirectory() (string, error) {
	name := "Burrow"
	if !mygo.App.IsPackaged() || strings.HasSuffix(mygo.App.Name(), " Dev") {
		name = "Burrow Dev"
	}
	mygo.App.SetName(name)
	var base string
	var err error
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", errors.New("LOCALAPPDATA is unavailable")
		}
	} else if runtime.GOOS == "linux" {
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base, err = os.UserHomeDir()
			base = filepath.Join(base, ".local", "share")
		}
	} else {
		base, err = os.UserConfigDir()
	}
	if err != nil {
		return "", err
	}
	directory := filepath.Join(base, name)
	mygo.App.SetPath(mygo.PathUserData, directory)
	if runtime.GOOS == "windows" {
		mygo.App.SetPath(mygo.PathCache, filepath.Join(directory, "cache"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(directory, "logs"))
	}
	return directory, nil
}

func credentialStore() mygo.SecretsModule {
	service := "dev.burrow.app"
	if mygo.App.Name() == "Burrow Dev" {
		service += ".dev"
	}
	return mygo.Secrets.Service(service)
}
