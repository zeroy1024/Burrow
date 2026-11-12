package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

const (
	updateRepository = "zeroy1024/Burrow"
	updateGitHub     = "https://github.com/"
	updateGitHubAPI  = "https://api.github.com"
)

// directTransport ignores both the system proxy and the proxy environment.
var directTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}()

// updateSource serves the manifests and archives that the release workflow
// publishes to GitHub Releases. MyGo still verifies every download against
// the public key built into the app, so a mirror cannot change its content.
type updateSource struct {
	systemProxy bool
	// mirror is prefixed to github.com URLs, e.g. https://mirror.example/.
	mirror string
}

func (s updateSource) Manifest(ctx context.Context, prereleases bool) (io.ReadCloser, error) {
	name := "update-" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
	release := "latest/download"
	if prereleases {
		// The latest release never is a prerelease; the newest listed one may be.
		body, err := s.get(ctx, updateGitHubAPI+"/repos/"+updateRepository+"/releases?per_page=1")
		if err != nil {
			return nil, err
		}
		var releases []struct {
			TagName string `json:"tag_name"`
		}
		err = json.NewDecoder(io.LimitReader(body, 16<<20)).Decode(&releases)
		body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading the releases of %s: %w", updateRepository, err)
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("%s has no published release", updateRepository)
		}
		release = "download/" + url.PathEscape(releases[0].TagName)
	}
	return s.Open(ctx, updateGitHub+updateRepository+"/releases/"+release+"/"+name)
}

func (s updateSource) Open(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	if mirror := strings.TrimSpace(s.mirror); mirror != "" && strings.HasPrefix(rawURL, updateGitHub) {
		rawURL = strings.TrimSuffix(mirror, "/") + "/" + rawURL
	}
	return s.get(ctx, rawURL)
}

func (s updateSource) get(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if target.Scheme != "https" {
		return nil, fmt.Errorf("refusing to download %s: updates need HTTPS", rawURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// GitHub's API rejects requests without a User-Agent.
	request.Header.Set("User-Agent", "burrow-updater")
	transport := directTransport
	if s.systemProxy {
		transport = netproxy.Transport()
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", rawURL, response.Status)
	}
	return response.Body, nil
}

// applyUpdatePreferences makes later checks use the current channel, proxy
// and mirror. A check already under way keeps the source it started with.
func (a *app) applyUpdatePreferences() {
	mygo.Updater.SetPrereleases(a.prefs.UpdateChannel == "preview")
	mygo.Updater.SetSource(updateSource{systemProxy: a.prefs.UpdateSystemProxy, mirror: a.prefs.UpdateMirror})
}

func (a *app) updateSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "UpdateTitle", locales.UpdateTitle(), "", []settingField{
		{"UpdateCheck", locales.UpdateCheck(), fmt.Sprintf(locales.UpdateCurrentVersion(), appVersion()), "", func() {
			uikit.Button(c, locales.UpdateCheck(), uikit.ButtonProps{}).OnClick(updater.CheckForUpdates)
		}},
		{"UpdateAutomatic", locales.UpdateAutomatic(), locales.UpdateAutomaticDetail(), "", func() {
			p := t.C.Switch
			automatic := updater.AutomaticChecks()
			if uikit.Switch(c, &automatic, uikit.ToggleProps{
				Label: locales.UpdateAutomatic(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}).Size(p.W, p.H).Label(locales.UpdateAutomatic()).Changed() {
				updater.SetAutomaticChecks(automatic)
			}
		}},
		{"UpdateChannel", locales.UpdateChannel(), locales.UpdateChannelDetail(), "", func() {
			values := []string{"stable", "preview"}
			settingsSegment(c, "update-channel", locales.UpdateChannel(),
				[]string{locales.UpdateChannelStable(), locales.UpdateChannelPreview()},
				slices.Index(values, a.prefs.UpdateChannel), func(index int) {
					a.prefs.UpdateChannel = values[index]
					a.updatePreferencesChanged()
				})
		}},
		{"UpdateSystemProxy", locales.UpdateSystemProxy(), locales.UpdateSystemProxyDetail(), "", func() {
			p := t.C.Switch
			if uikit.Switch(c, &a.prefs.UpdateSystemProxy, uikit.ToggleProps{
				Label: locales.UpdateSystemProxy(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}).Size(p.W, p.H).Label(locales.UpdateSystemProxy()).Changed() {
				a.updatePreferencesChanged()
			}
		}},
		{"UpdateMirror", locales.UpdateMirror(), locales.UpdateMirrorDetail(), "", func() {
			if uikit.Input(c, &a.prefs.UpdateMirror, uikit.InputProps{
				Mono: true, Label: locales.UpdateMirror(), Placeholder: "https://",
			}).Width(t.C.Settings.SelectWidth).Changed() {
				a.updatePreferencesChanged()
			}
		}},
	})
}

func (a *app) updatePreferencesChanged() {
	a.applyUpdatePreferences()
	a.preferencesChanged()
}
