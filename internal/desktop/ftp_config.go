package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type filePreferencesState struct {
	path    string
	loadErr error
}

type filePreferences struct {
	LocalPaneOpen         bool              `json:"localPaneOpen"`
	LocalRatio            float32           `json:"localRatio"`
	LocalPaths            map[string]string `json:"localPaths"`
	LocalPathKeysVersion  int               `json:"localPathKeysVersion"`
	TransferDockCollapsed bool              `json:"transferDockCollapsed"`
	TransferFilter        int               `json:"transferFilter"`
	TransferDockHeight    float32           `json:"transferDockHeight,omitempty"`
}

func (a *app) fileSiteKey(session *fileSession) string {
	return a.hosts[session.host].session.ID
}

func (a *app) restoreFileSession(session *fileSession) {
	local := a.prefs.DefaultLocalDir
	if options := session.config.Files; options != nil {
		if options.LocalDir != "" {
			local = options.LocalDir
		}
		if options.RemoteDir != "" {
			session.remote.path = options.RemoteDir
			session.remote.history = []string{options.RemoteDir}
		}
	}
	if local != "" {
		session.local.path = local
		session.local.history = []string{local}
	}
	if local := a.files.localPaths[a.fileSiteKey(session)]; local != "" {
		session.local.path = local
		session.local.history = []string{local}
	}
}

func (a *app) loadFilePreferences() error {
	err := a.readFilePreferences()
	a.filePrefs.loadErr = err
	return err
}

func (a *app) readFilePreferences() error {
	if a.catalogConfig.path == "" {
		return fmt.Errorf("主机配置目录不可用")
	}
	a.filePrefs.path = filepath.Join(filepath.Dir(a.catalogConfig.path), "files.json")
	data, err := os.ReadFile(a.filePrefs.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	prefs := filePreferences{LocalRatio: theme.Current().C.FTPSplit.DefaultRatio}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return err
	}
	if prefs.LocalRatio <= 0 || prefs.LocalRatio >= 1 {
		return fmt.Errorf("localRatio 必须介于 0 与 1 之间")
	}
	if prefs.TransferFilter < 0 || prefs.TransferFilter > 2 {
		return fmt.Errorf("无效的传输筛选项")
	}
	if prefs.TransferDockHeight < 0 {
		return fmt.Errorf("transferDockHeight 不能为负数")
	}
	if prefs.LocalPathKeysVersion == 0 && a.catalogConfig.loadErr != nil {
		return fmt.Errorf("会话配置读取失败，无法迁移站点本地路径：%w", a.catalogConfig.loadErr)
	}
	a.files.localOpen, a.files.ratio = prefs.LocalPaneOpen, prefs.LocalRatio
	if prefs.LocalPaths != nil {
		a.files.localPaths = prefs.LocalPaths
		if prefs.LocalPathKeysVersion == 0 {
			for _, h := range a.hosts {
				if h.protocol != "sftp" && h.protocol != "ftp" {
					continue
				}
				if _, exists := a.files.localPaths[h.session.ID]; exists {
					continue
				}
				address := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
				if path, exists := a.files.localPaths[address]; exists {
					a.files.localPaths[h.session.ID] = path
				}
			}
		}
	}
	a.transfers.collapsed, a.transfers.filter = prefs.TransferDockCollapsed, prefs.TransferFilter
	a.transfers.dockHeight = prefs.TransferDockHeight
	// Finish migration before newly created sites could inherit an old address key.
	if prefs.LocalPathKeysVersion == 0 {
		if err := a.writeFilePreferences(); err != nil {
			return fmt.Errorf("迁移站点本地路径失败：%w", err)
		}
	}
	return nil
}

func (a *app) saveFilePreferences(c *ui.Context) {
	if err := a.writeFilePreferences(); err != nil {
		uikit.Toast(c, uikit.ToastProps{ID: "保存文件传输偏好失败：" + err.Error(), Tone: uikit.ToneDanger, Title: "保存文件传输偏好失败", Detail: err.Error()})
	}
}

func (a *app) writeFilePreferences() error {
	if a.filePrefs.loadErr != nil {
		return fmt.Errorf("原配置读取失败，已阻止覆盖：%w", a.filePrefs.loadErr)
	}
	if a.filePrefs.path == "" {
		return fmt.Errorf("配置目录不可用")
	}
	prefs := filePreferences{
		LocalPaneOpen: a.files.localOpen, LocalRatio: a.files.ratio, LocalPaths: a.files.localPaths,
		LocalPathKeysVersion:  1,
		TransferDockCollapsed: a.transfers.collapsed, TransferFilter: a.transfers.filter,
		TransferDockHeight: a.transfers.dockHeight,
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return writeCatalog(a.filePrefs.path, append(data, '\n'))
}
