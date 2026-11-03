package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
)

type sessionConfig struct {
	ID               string       `json:"id"`
	Auth             string       `json:"auth"`
	KeyID            string       `json:"keyId,omitempty"`
	PortEdited       bool         `json:"portEdited"`
	SSH              *sshOptions  `json:"ssh,omitempty"`
	Files            *fileOptions `json:"files,omitempty"`
	PasswordSaved    bool         `json:"passwordSaved,omitempty"`
	PasswordRevision string       `json:"passwordRevision,omitempty"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

type sshOptions struct {
	JumpHostID  string   `json:"jumpHostId,omitempty"`
	KeepAlive   int      `json:"keepAlive"`
	InitCommand string   `json:"initCommand,omitempty"`
	TermScheme  string   `json:"termScheme,omitempty"`
	Env         []string `json:"env,omitempty"`
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
	Path        string    `json:"path"`
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

// Credentials are revision-bound to immutable endpoint snapshots.
func deleteSessionCredentials(id string) error {
	return credentialStore().Delete(sessionCredentialKey(id))
}

// Secret JSON lives only in the credential store; configuration records keep
// the revision that binds an immutable endpoint snapshot to one saved password.
func (a *app) storeSessionSecrets(h *host, password string, changed bool, passphrase string, passphraseChanged bool) error {
	initializeSession(h)
	if changed {
		var err error
		revision := ""
		if password == "" {
			err = deleteSessionCredentials(h.session.ID)
		} else {
			revision = newSessionID()
			var data []byte
			data, err = json.Marshal(sessionCredential{Revision: revision, Secret: password})
			if err == nil {
				err = credentialStore().Set(sessionCredentialKey(h.session.ID), data)
			}
			clear(data)
		}
		if err != nil {
			return fmt.Errorf("系统钥匙串：%w", err)
		}
		h.session.PasswordSaved = password != ""
		h.session.PasswordRevision = revision
	}
	if passphraseChanged && h.session.KeyID != "" {
		key := "key/" + h.session.KeyID + "/passphrase"
		var err error
		if passphrase == "" {
			err = credentialStore().Delete(key)
		} else {
			err = credentialStore().Set(key, []byte(passphrase))
		}
		if err != nil {
			return fmt.Errorf("保存私钥口令：%w", err)
		}
	}
	h.session.UpdatedAt = time.Now().UTC()
	return nil
}

func (a *app) sessionPassword(h host) (string, error) {
	if !h.session.PasswordSaved {
		return "", nil
	}
	data, err := credentialStore().Get(sessionCredentialKey(h.session.ID))
	if err == nil {
		defer clear(data)
		if h.session.PasswordRevision == "" {
			return "", fmt.Errorf("保存的密码已更新，请为此会话重新输入：%w", mygo.ErrSecretNotFound)
		}
		var credential sessionCredential
		if err := json.Unmarshal(data, &credential); err != nil || credential.Revision == "" || credential.Secret == "" {
			return "", errors.New("系统钥匙串中的会话凭据格式无效")
		}
		if credential.Revision != h.session.PasswordRevision {
			return "", fmt.Errorf("保存的密码与此会话快照不匹配，请重新输入：%w", mygo.ErrSecretNotFound)
		}
		return credential.Secret, nil
	}
	if !errors.Is(err, mygo.ErrSecretNotFound) {
		return "", fmt.Errorf("读取会话密码：%w", err)
	}
	return "", fmt.Errorf("此会话保存的密码已不可用，请重新输入：%w", mygo.ErrSecretNotFound)
}

// Keep the previous credential values until the atomic catalog write commits,
// so a disk error does not silently change credentials of an existing session.
func snapshotSessionSecrets(h host, passwordChanged, passphraseChanged bool) (func() error, func(), error) {
	keys := []string{}
	if passwordChanged {
		keys = append(keys, sessionCredentialKey(h.session.ID))
	}
	if passphraseChanged && h.session.KeyID != "" {
		keys = append(keys, "key/"+h.session.KeyID+"/passphrase")
	}
	values := make(map[string][]byte, len(keys))
	release := func() {
		for _, data := range values {
			clear(data)
		}
	}
	for _, key := range keys {
		data, err := credentialStore().Get(key)
		if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
			release()
			return nil, nil, err
		}
		if err == nil {
			values[key] = data
		}
	}
	rollback := func() error {
		var result error
		for _, key := range keys {
			var err error
			if data, ok := values[key]; ok {
				err = credentialStore().Set(key, data)
			} else {
				err = credentialStore().Delete(key)
			}
			result = errors.Join(result, err)
		}
		return result
	}
	return rollback, release, nil
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
		return managedKey{}, errors.New("请选择私钥")
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
		seen := map[string]bool{h.session.ID: true}
		for next := options.JumpHostID; next != ""; {
			jump, ok := byID[next]
			if !ok || jump.protocol != "" || jump.session.SSH == nil {
				return errors.New("invalid SSH jump host")
			}
			if seen[next] {
				return errors.New("SSH jump host cycle")
			}
			seen[next] = true
			next = jump.session.SSH.JumpHostID
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
