package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"sshdesk/internal/localfs"
)

func (a *app) exportManagedKey(key managedKey) {
	parent := a.window
	a.credentialWork(func(ctx context.Context) (result credentialResult) {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出私钥", DefaultPath: key.Name, Message: "导出的文件包含私钥，请妥善保管。"})
		if err != nil {
			return credentialResult{err: err}
		}
		if path == "" || ctx.Err() != nil {
			return credentialResult{}
		}
		data, err := a.vault.Get(ctx, key.SecretID)
		if err != nil {
			return credentialResult{err: err}
		}
		defer clear(data)
		// The native save dialog confirms overwrites; never follow an existing
		// symlink or special file to a different destination.
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() {
				return credentialResult{err: errors.New("导出目标必须是普通文件")}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return credentialResult{err: err}
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".burrow-key-export-*")
		if err != nil {
			return credentialResult{err: err}
		}
		temporary := file.Name()
		defer func() {
			if temporary != "" {
				if err := mygo.Shell.TrashItem(temporary); err != nil {
					result.err = errors.Join(result.err, fmt.Errorf("回收导出暂存文件：%w", err))
				}
			}
		}()
		if err := localfs.Protect(file); err != nil {
			return credentialResult{err: errors.Join(err, file.Close())}
		}
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return credentialResult{err: fmt.Errorf("导出私钥：%w", err)}
		}
		if err := os.Rename(temporary, path); err != nil {
			return credentialResult{err: err}
		}
		temporary = ""
		return credentialResult{notice: "私钥已导出"}
	})
}
