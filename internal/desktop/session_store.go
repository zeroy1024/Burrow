package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/vault"
)

type sessionConfig struct {
	Gateway          string        `json:"gateway,omitempty"`
	GatewayRoute     *[]gatewayHop `json:"gatewayRoute,omitempty"`
	ID               string        `json:"id"`
	Auth             string        `json:"auth"`
	KeyID            string        `json:"keyId,omitempty"`
	PortEdited       bool          `json:"portEdited"`
	SSH              *sshOptions   `json:"ssh,omitempty"`
	Files            *fileOptions  `json:"files,omitempty"`
	PasswordSaved    bool          `json:"passwordSaved,omitempty"`
	PasswordRevision string        `json:"passwordRevision,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
}

type sshOptions struct {
	KeepAlive   int      `json:"keepAlive"`
	InitCommand string   `json:"initCommand,omitempty"`
	TermScheme  string   `json:"termScheme,omitempty"`
	Env         []string `json:"env,omitempty"`
	// Platform pins the remote system; empty detects it on connect.
	Platform string `json:"platform,omitempty"`
}

type fileOptions struct {
	RemoteDir string `json:"remoteDir,omitempty"`
	LocalDir  string `json:"localDir,omitempty"`
	Encoding  string `json:"encoding"`
	FTPMode   string `json:"ftpMode,omitempty"`
	FTPTLS    string `json:"ftpTls,omitempty"`
}

type managedKey struct {
	PublicKey   []byte    `json:"publicKey"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Fingerprint string    `json:"fingerprint"`
	SecretID    string    `json:"secretId"`
	Comment     string    `json:"comment,omitempty"`
	Bits        int       `json:"bits"`
	CreatedAt   time.Time `json:"createdAt"`
}

func newSessionID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	data[6], data[8] = data[6]&15|64, data[8]&63|128
	h := hex.EncodeToString(data[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func initializeSession(h *host) {
	if h.system == "" {
		h.os = linuxSystem
	}
	if h.session.ID == "" {
		h.session.ID = newSessionID()
	}
	if h.session.Auth == "" {
		h.session.Auth = "password"
	}
	if h.session.CreatedAt.IsZero() {
		h.session.CreatedAt = time.Now().UTC()
	}
	if h.protocol == "" || h.protocol == "ssh" {
		h.protocol = ""
		if h.session.SSH == nil {
			h.session.SSH = &sshOptions{KeepAlive: 30}
		}
		h.session.Files = nil
	} else {
		h.session.SSH = nil
		if h.session.Files == nil {
			h.session.Files = &fileOptions{Encoding: "UTF-8"}
		}
		if h.protocol == "ftp" {
			if h.session.Files.FTPMode == "" {
				h.session.Files.FTPMode = "passive"
			}
			if h.session.Files.FTPTLS == "" {
				h.session.Files.FTPTLS = "explicit"
			}
		}
	}
}

type sessionCredential struct {
	Revision string `json:"revision"`
	Secret   string `json:"secret"`
}

func sessionCredentialKey(id string) string { return "session/" + id + "/credential" }

func privateCredentialKey(id string) string { return "key/" + id + "/private" }

// The caller commits these ciphertext changes with the catalog in one transaction.
func (a *app) prepareSessionSecrets(h *host, password string, changed bool, passphrase string, passphraseChanged bool) (*store.VaultChanges, error) {
	initializeSession(h)
	items := []vault.PlainItem{}
	deleted := []string{}
	defer func() {
		for _, item := range items {
			clear(item.Data)
		}
	}()
	if changed {
		revision := ""
		if password == "" {
			deleted = append(deleted, sessionCredentialKey(h.session.ID))
		} else {
			revision = newSessionID()
			data, err := json.Marshal(sessionCredential{Revision: revision, Secret: password})
			if err != nil {
				return nil, err
			}
			items = append(items, vault.PlainItem{ID: sessionCredentialKey(h.session.ID), Kind: "session_password", Revision: revision, Data: data})
		}
		h.session.PasswordSaved = password != ""
		h.session.PasswordRevision = revision
	}
	if passphraseChanged && h.session.KeyID != "" {
		id := "key/" + h.session.KeyID + "/passphrase"
		if passphrase == "" {
			deleted = append(deleted, id)
		} else {
			items = append(items, vault.PlainItem{ID: id, Kind: "key_passphrase", Revision: newSessionID(), Data: []byte(passphrase)})
		}
	}
	h.session.UpdatedAt = time.Now().UTC()
	return a.vault.Prepare(items, deleted)
}

func (a *app) sessionPassword(h host) (string, error) {
	if !h.session.PasswordSaved {
		return "", nil
	}
	data, err := a.vault.Get(a.ctx, sessionCredentialKey(h.session.ID))
	if err == nil {
		defer clear(data)
		if h.session.PasswordRevision == "" {
			return "", locales.Errorf(locales.MessageTheSavedPasswordHasChangedEnterItAgainForThisSession, vault.ErrNotFound)
		}
		var credential sessionCredential
		if err := json.Unmarshal(data, &credential); err != nil || credential.Revision == "" || credential.Secret == "" {
			return "", locales.NewError(locales.MessageInvalidSavedSessionCredentialFormat)
		}
		if credential.Revision != h.session.PasswordRevision {
			return "", locales.Errorf(
				locales.MessageTheSavedPasswordDoesNotMatchThisSessionSnapshotEnterItAgain,
				vault.ErrNotFound,
			)
		}
		return credential.Secret, nil
	}
	if !errors.Is(err, vault.ErrNotFound) {
		return "", locales.Errorf(locales.MessageReadSessionPassword, err)
	}
	return "", locales.Errorf(
		locales.MessageTheSavedPasswordForThisSessionIsNoLongerAvailableEnterItAgain,
		vault.ErrNotFound,
	)
}

func (a *app) modalAdvancedOpen(protocol string) bool {
	return a.prefs.ModalAdvancedOpen[protocol]
}

func (a *app) setModalAdvancedOpen(protocol string, open bool) {
	if a.prefs.ModalAdvancedOpen == nil {
		a.prefs.ModalAdvancedOpen = map[string]bool{}
	}
	a.prefs.ModalAdvancedOpen[protocol] = open
	a.queuePreferences()
}

func (a *app) keyByID(id string) (managedKey, error) {
	index := slices.IndexFunc(a.keys, func(k managedKey) bool { return k.ID == id })
	if index == -1 {
		return managedKey{}, locales.NewError(locales.MessageSelectAPrivateKey)
	}
	return a.keys[index], nil
}

func validateSessionCatalog(hosts []host, keys []managedKey) error {
	keyIDs := map[string]bool{}
	for _, k := range keys {
		if k.ID == "" || k.ID == "." || k.ID == ".." || keyIDs[k.ID] || filepath.Base(k.ID) != k.ID {
			return errors.New("invalid or duplicate managed key id")
		}
		keyIDs[k.ID] = true
	}
	byID := map[string]host{}
	for _, h := range hosts {
		if h.session.ID == "" {
			return errors.New("empty session id")
		}
		if _, exists := byID[h.session.ID]; exists {
			return errors.New("duplicate session id")
		}
		byID[h.session.ID] = h
	}
	for _, h := range hosts {
		if err := validateSessionProfile(h, byID, keyIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateSessionProfile(h host, byID map[string]host, keyIDs map[string]bool) error {
	if h.session.ID == "" {
		return errors.New("empty session id")
	}
	if !slices.Contains([]string{"password", "key", "agent", "anonymous"}, h.session.Auth) {
		return errors.New("invalid authentication method")
	}
	if h.protocol == "ftp" && h.session.Auth != "password" && h.session.Auth != "anonymous" {
		return errors.New("invalid FTP authentication method")
	}
	if h.protocol != "ftp" && h.session.Auth == "anonymous" {
		return errors.New("anonymous authentication requires FTP")
	}
	if h.session.Auth == "key" && !keyIDs[h.session.KeyID] {
		return errors.New("session references a missing private key")
	}
	if options := h.session.SSH; options != nil {
		if options.KeepAlive < 0 || options.KeepAlive > 3600 {
			return errors.New("invalid keepalive interval")
		}
		if !validPlatformOverride(options.Platform) {
			return errors.New("invalid remote platform")
		}
	}
	if h.session.GatewayRoute != nil {
		if err := validateGatewayRoute(*h.session.GatewayRoute); err != nil {
			return err
		}
	}
	if options := h.session.Files; options != nil {
		if !slices.Contains([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}, options.Encoding) {
			return errors.New("invalid filename encoding")
		}
		if h.protocol == "ftp" && (!slices.Contains([]string{"passive", "active"}, options.FTPMode) || !slices.Contains([]string{"none", "explicit", "implicit"}, options.FTPTLS)) {
			return errors.New("invalid FTP options")
		}
	}
	return nil
}

func validPlatformOverride(platform string) bool {
	return platform == "" || slices.Contains(hostinfo.Overrides, hostinfo.Platform(platform))
}
