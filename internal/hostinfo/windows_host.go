package hostinfo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
)

const windowsFactsMarker = "__BURROW_WINDOWS_FACTS_V1__"

// windowsFactsScript runs in Windows PowerShell 5.1, which every Win32-OpenSSH
// host has. It reports failures inside its own output so nothing reaches
// stderr, and prints base64 so the console code page cannot corrupt localized
// names. Caption and DisplayVersion follow the system language.
const windowsFactsScript = `$ProgressPreference = 'SilentlyContinue'
$ErrorActionPreference = 'Stop'
try {
  $os = Get-CimInstance -ClassName Win32_OperatingSystem
  $cv = Get-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
  $openssh = Get-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -ErrorAction SilentlyContinue
  $arch = $env:PROCESSOR_ARCHITEW6432
  if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
  $facts = [ordered]@{
    hostname = [System.Net.Dns]::GetHostName()
    caption = [string]$os.Caption
    version = [string]$os.Version
    ubr = [string]$cv.UBR
    display = [string]$cv.DisplayVersion
    release = [string]$cv.ReleaseId
    arch = [string]$arch
    shell = [string]$openssh.DefaultShell
  }
} catch {
  $facts = [ordered]@{ error = $_.Exception.Message }
}
$json = ConvertTo-Json -InputObject $facts -Compress
[Console]::Out.Write('` + windowsFactsMarker + `' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json)))
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
	_, encoded, found := strings.Cut(output, windowsFactsMarker)
	if !found {
		return Facts{}, errors.New("服务器返回了无法识别的 Windows 基础信息")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return Facts{}, errors.New("服务器返回了无法识别的 Windows 基础信息")
	}
	var raw struct {
		Hostname, Caption, Version, UBR, Display, Release, Arch, Shell, Error string
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Facts{}, errors.New("服务器返回了无法识别的 Windows 基础信息")
	}
	if raw.Error != "" {
		return Facts{}, errors.New("读取 Windows 基础信息失败：" + raw.Error)
	}
	facts := Facts{Platform: Windows, OSID: "windows", OSName: strings.TrimSpace(raw.Caption), Hostname: raw.Hostname,
		OSVersion: raw.Display, Kernel: raw.Version, Arch: raw.Arch, Shell: windowsShell(raw.Shell), CollectedAt: collectedAt}
	if facts.OSName == "" {
		facts.OSName = Windows.DisplayName()
	}
	if facts.OSVersion == "" {
		facts.OSVersion = raw.Release
	}
	if raw.UBR != "" && facts.Kernel != "" {
		facts.Kernel += "." + raw.UBR
	}
	if !facts.Valid() {
		return Facts{}, errors.New("无法读取 Windows 主机名或系统版本")
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
