// Package hostinfo collects basic facts and live metrics from a remote host
// over its existing SSH transport, with one implementation per platform.
//
// Platform files are named *_host.go on purpose: a *_linux.go or *_windows.go
// suffix would make Go compile them only when building for that local OS,
// while every build must be able to read every kind of remote host.
package hostinfo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Platform string

const (
	Linux   Platform = "linux"
	Darwin  Platform = "darwin"
	Windows Platform = "windows"
	// Other is a recognized but unsupported system, such as FreeBSD.
	Other Platform = "other"
)

// Overrides lists the platforms a user can pin for a connection.
var Overrides = []Platform{Linux, Darwin, Windows}

func (p Platform) DisplayName() string {
	switch p {
	case Linux:
		return "Linux"
	case Darwin:
		return "macOS"
	case Windows:
		return "Windows"
	}
	return "其他系统"
}

// ErrUnsupported reports a platform without monitoring support.
var ErrUnsupported = errors.New("该系统暂不支持监控")

// Runner is the access a collector needs to one remote host.
type Runner interface {
	Exec(ctx context.Context, command string) ([]byte, error)
	// Files opens a short-lived view of the remote file system (SFTP).
	Files(ctx context.Context) (Files, error)
}

type Files interface {
	StatFilesystem(ctx context.Context, name string) (Usage, error)
	ReadFile(ctx context.Context, name string, limit int64) ([]byte, error)
	Close() error
}

type Usage struct{ Total, Free uint64 }

// Facts are the slowly changing host details shown with a connection. The JSON
// names are persisted; records saved before Platform existed are Linux.
type Facts struct {
	Platform  Platform `json:"platform,omitempty"`
	Hostname  string   `json:"hostname"`
	OSID      string   `json:"osId"`
	OSVersion string   `json:"osVersion"`
	OSName    string   `json:"osName"`
	Kernel    string   `json:"kernel"`
	Arch      string   `json:"arch,omitempty"`
	// Shell is the default shell of a Windows OpenSSH server: cmd, powershell,
	// pwsh or another program's base name.
	Shell       string    `json:"shell,omitempty"`
	CollectedAt time.Time `json:"collectedAt"`
}

// Valid requires what each collector always reports. Other systems only have
// what uname happened to print.
func (f Facts) Valid() bool {
	if f.OSName == "" || f.CollectedAt.IsZero() || !f.Platform.known() {
		return false
	}
	return f.Platform == Other || f.Hostname != "" && f.Kernel != ""
}

func (p Platform) known() bool {
	return p == Linux || p == Darwin || p == Windows || p == Other
}

// Detection is what identifying the platform revealed. System, Hostname and
// Release come from uname when it ran.
type Detection struct {
	Platform                  Platform
	System, Hostname, Release string
}

type Collector interface {
	Facts(ctx context.Context, r Runner, d Detection) (Facts, error)
	// NewSampler starts monitoring one transport. The sampler keeps whatever
	// state its platform needs to turn counters into rates.
	NewSampler(r Runner) Sampler
}

type Sampler interface {
	// Sample takes one reading. Calls must not overlap.
	Sample(ctx context.Context) (Metrics, error)
	// Disk reads root file-system usage for the root a sample identified by
	// Metrics.DiskKey. It keeps no sampler state and may run alongside Sample.
	Disk(ctx context.Context, key string) (Usage, error)
}

// Metrics is one normalized reading. Nil values were not measured; their
// names are listed in Unavailable.
type Metrics struct {
	CPU, Memory, Uptime, Down, Up *float64
	Cores                         int
	Load                          string
	MemoryUsed, MemoryTotal       uint64
	Processes                     *int
	Top                           []Process
	// DiskKey identifies the root file system; Disk results apply only to a
	// sample with the same key. Empty means disk usage cannot be attributed.
	DiskKey, DiskDevice, Filesystem string
	Unavailable                     []string
}

type Process struct {
	Name string
	CPU  float64
}

func For(p Platform) Collector {
	switch p {
	case Linux:
		return linuxCollector{}
	case Darwin:
		return darwinCollector{}
	case Windows:
		return windowsCollector{}
	}
	return unsupported{}
}

// windowsBanner marks Win32-OpenSSH. A server may advertise any version
// string, so it is a hint that avoids running uname under cmd.exe.
const windowsBanner = "OpenSSH_for_Windows"

// Detect identifies the remote platform from the SSH banner and uname. A
// Windows default shell of WSL or Cygwin reports that environment; users pin
// the platform for such hosts.
func Detect(ctx context.Context, r Runner, banner string) (Detection, error) {
	if strings.Contains(banner, windowsBanner) {
		return Detection{Platform: Windows, System: "Windows"}, nil
	}
	output, err := r.Exec(ctx, "uname -snr")
	if err != nil {
		return Detection{}, fmt.Errorf("无法识别远端系统，可在会话设置中手动指定：%w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return Detection{}, errors.New("无法识别远端系统，可在会话设置中手动指定：uname 无输出")
	}
	d := Detection{System: fields[0], Platform: Other}
	if len(fields) > 1 {
		d.Hostname = fields[1]
	}
	if len(fields) > 2 {
		d.Release = fields[2]
	}
	switch upper := strings.ToUpper(fields[0]); {
	case fields[0] == "Linux":
		d.Platform = Linux
	case fields[0] == "Darwin":
		d.Platform = Darwin
	case strings.HasPrefix(upper, "MINGW"), strings.HasPrefix(upper, "MSYS"), strings.HasPrefix(upper, "CYGWIN"):
		// Git for Windows or Cygwin on PATH answered for a Windows host.
		d.Platform, d.Hostname, d.Release = Windows, "", ""
	}
	return d, nil
}

// Collect identifies the platform, unless override pins one, and gathers its
// facts.
func Collect(ctx context.Context, r Runner, banner string, override Platform) (Facts, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d := Detection{Platform: override}
	if override == "" {
		var err error
		if d, err = Detect(ctx, r, banner); err != nil {
			// Without uname, a Windows host behind a rewritten banner still
			// answers PowerShell.
			if facts, windowsErr := (windowsCollector{}).Facts(ctx, r, Detection{Platform: Windows}); windowsErr == nil {
				return facts, nil
			}
			return Facts{}, err
		}
	}
	return For(d.Platform).Facts(ctx, r, d)
}

// unsupported records what detection found for another system so its name is
// shown, and reports ErrUnsupported for monitoring.
type unsupported struct{}

func (unsupported) Facts(_ context.Context, _ Runner, d Detection) (Facts, error) {
	name := d.Platform.DisplayName()
	if d.Platform == Other && d.System != "" {
		name = d.System
	}
	return Facts{Platform: d.Platform, Hostname: d.Hostname, Kernel: d.Release, OSID: strings.ToLower(name),
		OSName: name, CollectedAt: time.Now().UTC()}, nil
}

func (unsupported) NewSampler(Runner) Sampler { return unsupportedSampler{} }

// unsupportedSampler stands in until a platform has monitoring.

type unsupportedSampler struct{}

func (unsupportedSampler) Sample(context.Context) (Metrics, error) { return Metrics{}, ErrUnsupported }
func (unsupportedSampler) Disk(context.Context, string) (Usage, error) {
	return Usage{}, ErrUnsupported
}
