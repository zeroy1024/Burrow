package profile

import (
	"errors"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/wggateway"
)

// System IDs are stored in endpoint snapshots. Append new values before SystemCount.
const (
	SystemUbuntu = iota
	SystemDebian
	SystemCentOS
	SystemAlpine
	SystemFedora
	SystemLinux
	SystemArch
	SystemMacOS
	SystemWindows
	SystemRedHat
	SystemRockyLinux
	SystemAlmaLinux
	SystemOpenSUSE
	SystemKali
	SystemLinuxMint
	SystemRaspberryPiOS
	SystemDeepin
	SystemFreeBSD
	SystemCount
)

var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

func ValidHost(value string) bool {
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	if strings.HasPrefix(value, "[") || strings.HasSuffix(value, "]") {
		return strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") && net.ParseIP(value[1:len(value)-1]) != nil
	}
	if net.ParseIP(value) != nil {
		return true
	}
	if len(value) > 253 || strings.ContainsAny(value, ":[]/") {
		return false
	}
	value = strings.TrimSuffix(value, ".")
	for _, label := range strings.Split(value, ".") {
		if len(label) > 63 || !hostnamePattern.MatchString(label) {
			return false
		}
	}
	return true
}

func (config Config) Validate(protocol string, keyIDs map[string]bool) error {
	if config.ID == "" {
		return errors.New("empty session id")
	}
	if !slices.Contains([]string{"password", "key", "agent", "anonymous"}, config.Auth) {
		return errors.New("invalid authentication method")
	}
	if protocol == "ftp" && config.Auth != "password" && config.Auth != "anonymous" {
		return errors.New("invalid FTP authentication method")
	}
	if protocol != "ftp" && config.Auth == "anonymous" {
		return errors.New("anonymous authentication requires FTP")
	}
	if config.Auth == "key" && !keyIDs[config.KeyID] {
		return errors.New("session references a missing private key")
	}
	if options := config.SSH; options != nil {
		if options.KeepAlive < 0 || options.KeepAlive > 3600 {
			return errors.New("invalid keepalive interval")
		}
		if options.HealthCheck < 0 || options.HealthCheck > 3600 {
			return locales.NewError(locales.SshHealthCheckInvalid)
		}
		if !validPlatformOverride(options.Platform) {
			return errors.New("invalid remote platform")
		}
		if err := validateForwardRules(options.Forwards); err != nil {
			return err
		}
	}
	if config.GatewayRoute != nil {
		if err := ValidateRoute(*config.GatewayRoute); err != nil {
			return err
		}
	}
	if options := config.Files; options != nil {
		if !slices.Contains([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}, options.Encoding) {
			return errors.New("invalid filename encoding")
		}
		if protocol == "ftp" && (!slices.Contains([]string{"passive", "active"}, options.FTPMode) || !slices.Contains([]string{"none", "explicit", "implicit"}, options.FTPTLS)) {
			return errors.New("invalid FTP options")
		}
	}
	return nil
}

func validPlatformOverride(platform string) bool {
	return platform == "" || slices.Contains(hostinfo.Overrides, hostinfo.Platform(platform))
}
func validateForwardRules(rules []ForwardRule) error {
	ids := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.ID == "" || ids[rule.ID] {
			return errors.New("invalid or duplicate forwarding rule id")
		}
		ids[rule.ID] = true
		if _, err := rule.BindAddress(); err != nil {
			return err
		}
		if _, err := rule.Target(); err != nil {
			return err
		}
	}
	return nil
}
func ValidateTransferEndpoint(endpoint Endpoint, files bool) error {
	if strings.TrimSpace(endpoint.Name) == "" || strings.TrimSpace(endpoint.Host) == "" ||
		strings.ContainsAny(endpoint.Host+endpoint.User, "\r\n\x00") || endpoint.Port < 1 || endpoint.Port > 65535 ||
		endpoint.ID == "" || endpoint.OS < 0 || endpoint.OS >= SystemCount {
		return locales.NewError(locales.MessageInvalidTransferSessionSnapshot)
	}
	if files && endpoint.Protocol != "sftp" && endpoint.Protocol != "ftp" ||
		!files && endpoint.Protocol != "" && endpoint.Protocol != "sftp" {
		return locales.NewError(locales.MessageInvalidTransferSessionProtocol)
	}
	if !slices.Contains([]string{"password", "key", "agent", "anonymous"}, endpoint.Auth) ||
		endpoint.Protocol == "ftp" && endpoint.Auth != "password" && endpoint.Auth != "anonymous" ||
		endpoint.Protocol != "ftp" && endpoint.Auth == "anonymous" {
		return locales.NewError(locales.MessageInvalidTransferAuthenticationMethod)
	}
	if endpoint.Auth == "key" && (endpoint.KeyID == "" || endpoint.KeyID == "." || endpoint.KeyID == ".." ||
		filepath.Base(endpoint.KeyID) != endpoint.KeyID) {
		return locales.NewError(locales.MessageInvalidTransferPrivateKeyReference)
	}
	if options := endpoint.SSH; options != nil {
		if options.KeepAlive < 0 || options.KeepAlive > 3600 {
			return locales.NewError(locales.MessageInvalidTransferKeepalive)
		}
		if options.HealthCheck < 0 || options.HealthCheck > 3600 {
			return locales.NewError(locales.SshHealthCheckInvalid)
		}
		if !validPlatformOverride(options.Platform) {
			return locales.NewError(locales.MessageInvalidTransferRemoteSystem)
		}
	}
	if options := endpoint.Files; options != nil {
		if !slices.Contains([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}, options.Encoding) {
			return locales.NewError(locales.MessageInvalidTransferFilenameEncoding)
		}
		if endpoint.Protocol == "ftp" && (!slices.Contains([]string{"none", "explicit", "implicit"}, options.FTPTLS) ||
			!slices.Contains([]string{"passive", "active"}, options.FTPMode)) {
			return locales.NewError(locales.MessageInvalidFtpTransferOptions)
		}
	}
	if endpoint.GatewayRoute != nil {
		return ValidateRoute(*endpoint.GatewayRoute)
	}
	return nil
}

func SameTransferEndpoint(left, right Endpoint) bool {
	return left.Host == right.Host && left.Port == right.Port && left.User == right.User &&
		left.Auth == right.Auth && left.KeyID == right.KeyID && left.ID == right.ID
}

func ValidateRoute(route []GatewayHop) error {
	seen := map[string]bool{}
	for index, hop := range route {
		if hop.ID == "" || seen[hop.ID] {
			return locales.NewError(locales.MessageInvalidGatewaySnapshotIdOrRoutingLoop)
		}
		seen[hop.ID] = true
		switch hop.Kind {
		case "ssh":
			if hop.SSH == nil || hop.SOCKS != nil || hop.WG != nil || hop.SSH.Protocol != "" || hop.SSH.GatewayRoute != nil {
				return locales.NewError(locales.MessageInvalidSshGatewaySnapshot)
			}
			if err := ValidateTransferEndpoint(*hop.SSH, false); err != nil {
				return err
			}
		case "socks5":
			if hop.SSH != nil || hop.SOCKS == nil || hop.WG != nil {
				return locales.NewError(locales.MessageInvalidSocks5GatewaySnapshot)
			}
			if err := ValidateSOCKS(*hop.SOCKS); err != nil {
				return err
			}
		case "wireguard":
			if index != 0 || hop.WG == nil || hop.SSH != nil || hop.SOCKS != nil {
				return locales.NewError(locales.WgRouteFirst)
			}
			if err := ValidateWireGuard(*hop.WG); err != nil {
				return err
			}
		default:
			return locales.NewError(locales.MessageUnknownGatewaySnapshotType)
		}
	}
	return nil
}

func ValidateSOCKS(s SOCKSGateway) error {
	if !ValidHost(s.Address) || s.Port < 1 || s.Port > 65535 {
		return locales.NewError(locales.MessageInvalidSocks5AddressOrPort)
	}
	if len(s.Username) > 255 || s.PasswordSaved && (s.Username == "" || s.PasswordRevision == "") {
		return locales.NewError(locales.MessageInvalidSocks5AuthenticationInformation)
	}
	return nil
}

func ValidateWireGuard(config wggateway.Config) error {
	if config.CredentialRevision == "" {
		return locales.NewError(locales.WgMissingPrivateKey)
	}
	return config.Validate()
}
