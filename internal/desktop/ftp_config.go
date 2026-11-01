package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type filePreferencesState struct {
	path    string
	loadErr error
}

type filePreferences struct {
	LocalPaneOpen         bool              `json:"localPaneOpen"`
	LocalRatio            float32           `json:"localRatio"`
	LocalPaths            map[string]string `json:"localPaths,omitempty"`
	TransferDockCollapsed bool              `json:"transferDockCollapsed"`
	TransferFilter        int               `json:"transferFilter"`
	TransferDockHeight    float32           `json:"transferDockHeight,omitempty"`
}

func (a *app) restoreFileSession(session *fileSession) {
	if session.temporary {
		return
	}
	if local := a.fileCatalog.localPaths[session.host]; local != "" {
		session.local.path = local
		session.local.history = []string{local}
	}
}

func (a *app) rememberFileLocalPath(c *ui.Context, session *fileSession, path string) {
	if session.temporary {
		return
	}
	if _, exists := a.fileCatalog.hosts[session.host]; !exists {
		return
	}
	previous, existed := a.fileCatalog.localPaths[session.host]
	a.fileCatalog.localPaths[session.host] = path
	if err := a.saveCatalog(); err != nil {
		if existed {
			a.fileCatalog.localPaths[session.host] = previous
		} else {
			delete(a.fileCatalog.localPaths, session.host)
		}
		c.Toast("保存站点本地路径失败：" + err.Error())
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
	a.files.localOpen, a.files.ratio = prefs.LocalPaneOpen, prefs.LocalRatio
	a.transfers.collapsed, a.transfers.filter = prefs.TransferDockCollapsed, prefs.TransferFilter
	a.transfers.dockHeight = prefs.TransferDockHeight
	return nil
}

func (a *app) saveFilePreferences(c *ui.Context) {
	if err := a.writeFilePreferences(); err != nil {
		c.Toast("保存文件传输偏好失败：" + err.Error())
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
		LocalPaneOpen: a.files.localOpen, LocalRatio: a.files.ratio,
		TransferDockCollapsed: a.transfers.collapsed, TransferFilter: a.transfers.filter,
		TransferDockHeight: a.transfers.dockHeight,
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return writeCatalog(a.filePrefs.path, append(data, '\n'))
}
