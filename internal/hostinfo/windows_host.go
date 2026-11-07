package hostinfo

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/zeroy1024/Burrow/internal/locales"
)

const windowsFactsMarker = "__BURROW_WINDOWS_FACTS_V2__"

// windowsFactsScript runs on Windows PowerShell 2.0 and later, so it avoids
// Get-CimInstance, [ordered] and ConvertTo-Json. It reports failures inside its
// own output so nothing reaches stderr, and prints each value as base64 UTF-8
// so the console code page cannot corrupt localized names. Caption and
// DisplayVersion follow the system language.
const windowsFactsScript = `$ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = 'Stop'
function Field($name, $value) {
  $name + '=' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes([string]$value))
}
try {
  $os = Get-WmiObject -Class Win32_OperatingSystem
  $cv = Get-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
  $openssh = Get-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -ErrorAction SilentlyContinue
  $arch = $env:PROCESSOR_ARCHITEW6432
  if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
  $fields = @(
    (Field 'hostname' ([System.Net.Dns]::GetHostName())),
    (Field 'caption' $os.Caption),
    (Field 'version' $os.Version),
    (Field 'ubr' $cv.UBR),
    (Field 'display' $cv.DisplayVersion),
    (Field 'release' $cv.ReleaseId),
    (Field 'arch' $arch),
    (Field 'shell' $openssh.DefaultShell)
  )
} catch {
  $fields = @(Field 'error' $_.Exception.Message)
}
[Console]::Out.Write('` + windowsFactsMarker + `' + ($fields -join ';'))
`

// -EncodedCommand avoids quoting differences between cmd.exe and PowerShell
// as the server's default shell; powershell.exe also resolves from WSL/Cygwin.
var windowsFactsCommand = "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + encodePowerShell(windowsFactsScript)

func encodePowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	data := make([]byte, 0, len(units)*2)
	for _, unit := range units {
		data = append(data, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(data)
}

type windowsCollector struct{}

func (windowsCollector) Facts(ctx context.Context, r Runner, _ Detection) (Facts, error) {
	collectedAt := time.Now().UTC()
	output, err := r.Exec(ctx, windowsFactsCommand)
	if err != nil {
		return Facts{}, err
	}
	return parseWindowsFacts(string(output), collectedAt)
}

func (windowsCollector) NewSampler(Runner) Sampler { return unsupportedSampler{} }

func parseWindowsFacts(output string, collectedAt time.Time) (Facts, error) {
	invalid := locales.NewError(locales.MessageTheServerReturnedUnrecognizedWindowsInformation)
	_, encoded, found := strings.Cut(output, windowsFactsMarker)
	if !found {
		return Facts{}, invalid
	}
	raw := map[string]string{}
	for _, field := range strings.Split(strings.TrimSpace(encoded), ";") {
		name, value, ok := strings.Cut(field, "=")
		data, err := base64.StdEncoding.DecodeString(value)
		if !ok || err != nil || !utf8.Valid(data) {
			return Facts{}, invalid
		}
		raw[name] = string(data)
	}
	if raw["error"] != "" {
		return Facts{}, errors.New(locales.MessageUnableToReadWindowsInformation() + raw["error"])
	}
	facts := Facts{Platform: Windows, OSID: "windows", OSName: strings.TrimSpace(raw["caption"]), Hostname: raw["hostname"],
		OSVersion: raw["display"], Kernel: raw["version"], Arch: raw["arch"], Shell: windowsShell(raw["shell"]), CollectedAt: collectedAt}
	if facts.OSName == "" {
		facts.OSName = Windows.DisplayName()
	}
	if facts.OSVersion == "" {
		facts.OSVersion = raw["release"]
	}
	if raw["ubr"] != "" && facts.Kernel != "" {
		facts.Kernel += "." + raw["ubr"]
	}
	if !facts.Valid() {
		return Facts{}, locales.NewError(locales.MessageUnableToReadWindowsHostnameOrSystemVersion)
	}
	return facts, nil
}

// windowsShell names the OpenSSH DefaultShell; an unset value means cmd.exe.
func windowsShell(path string) string {
	if path == "" {
		return "cmd"
	}
	return normalizeProgram(path)
}

func normalizeProgram(path string) string {
	name := strings.ToLower(strings.Trim(path, `"' `))
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}
