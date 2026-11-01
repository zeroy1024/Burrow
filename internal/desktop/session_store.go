package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
)

type sessionConfig struct {
	ID            string       `json:"id"`
	Auth          string       `json:"auth"`
	KeyID         string       `json:"keyId,omitempty"`
	PortEdited    bool         `json:"portEdited"`
	SSH           *sshOptions  `json:"ssh,omitempty"`
	Files         *fileOptions `json:"files,omitempty"`
	PasswordSaved bool         `json:"passwordSaved,omitempty"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
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

// Secret values only cross the credential-store boundary; JSON keeps a presence bit.
func (a *app) storeSessionSecrets(h *host, password string, changed bool, passphrase string, passphraseChanged bool) error {
	initializeSession(h)
	if changed {
		key := "session/" + h.session.ID + "/password"
		var err error
		if password == "" {
			err = mygo.Secrets.Delete(key)
		} else {
			err = mygo.Secrets.Set(key, []byte(password))
		}
		if err != nil {
			return fmt.Errorf("系统钥匙串：%w", err)
		}
		h.session.PasswordSaved = password != ""
	}
	if passphraseChanged && h.session.KeyID != "" {
		key := "key/" + h.session.KeyID + "/passphrase"
		var err error
		if passphrase == "" {
			err = mygo.Secrets.Delete(key)
		} else {
			err = mygo.Secrets.Set(key, []byte(passphrase))
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
	data, err := mygo.Secrets.Get("session/" + h.session.ID + "/password")
	if err != nil {
		return "", fmt.Errorf("读取会话密码：%w", err)
	}
	return string(data), nil
}

// Keep the previous credential values until the atomic catalog write commits,
// so a disk error does not silently change credentials of an existing session.
func snapshotSessionSecrets(h host, passwordChanged, passphraseChanged bool) (func() error, func(), error) {
	keys := []string{}
	if passwordChanged {
		keys = append(keys, "session/"+h.session.ID+"/password")
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
		data, err := mygo.Secrets.Get(key)
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
				err = mygo.Secrets.Set(key, data)
			} else {
				err = mygo.Secrets.Delete(key)
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
		if k.ID == "" || keyIDs[k.ID] || filepath.Base(k.ID) != k.ID {
			return errors.New("invalid or duplicate managed key id")
		}
		keyIDs[k.ID] = true
	}
	byID := map[string]host{}
	for _, h := range hosts {
		byID[h.session.ID] = h
	}
	for _, h := range hosts {
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
	}
	return nil
}
