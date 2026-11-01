package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

var (
	errCatalogMigrationDeferred = errors.New("需要解锁旧私钥后才能导入会话")
	errCatalogMigrationCanceled = errors.New("旧会话导入已取消，原配置和私钥已保留")
	errCatalogMigrationBlocked  = errors.New("请先关闭主机或分组编辑窗口，并等待凭据操作完成后重试导入。")
)

// The candidate metadata and source paths are captured before a window exists.
// Each attempt decodes its own candidate; workers never inspect the live catalog.
type catalogMigrationPlan struct {
	directory, destination string
	metadata               []byte
	keySources             map[hostID]string
}

type catalogMigrationState struct {
	plan        *catalogMigrationPlan
	running     bool
	sequence    uint64
	unpublished []string
}

type preparedCatalogMigration struct {
	candidate *app
	data      []byte
	created   []string
}

func (a *app) catalogMigrationBusy() bool { return a.catalogConfig.migration.running }

func (a *app) catalogMigrationBlocked() bool {
	if a.credentials.busy || a.savingSession() {
		return true
	}
	for index := range a.pages {
		if a.pages[index].addHostOpen || a.pages[index].groupDialogOpen {
			return true
		}
	}
	return false
}

func (a *app) showCatalogMigrationError(c *ui.Context) {
	if a.catalogError == nil {
		return
	}
	props := uikit.ToastProps{
		ID: "catalog-migration-error", Title: "无法加载主机配置",
		Detail: a.catalogError.Error(), Tone: uikit.ToneDanger,
	}
	if state := &a.catalogConfig.migration; state.plan != nil && !state.running {
		props.Action, props.OnAction = "重试", a.startCatalogMigration
		props.Timeout = -1
	}
	uikit.Toast(c, props)
	a.catalogError = nil
}

// Called after the real window is ready, or by the existing error Toast action.
func (a *app) startCatalogMigration() {
	state := &a.catalogConfig.migration
	if state.plan == nil || state.running {
		return
	}
	if a.catalogMigrationBlocked() {
		a.catalogError = errCatalogMigrationBlocked
		a.window.Invalidate()
		return
	}
	plan := state.plan
	unpublished := slices.Clone(state.unpublished)
	state.running = true
	state.sequence++
	sequence := state.sequence
	ctx, cancel := context.WithCancel(a.ctx)
	a.sshWorkers.Go(func() {
		defer cancel()
		prepared, err := prepareCatalogMigration(ctx, plan, unpublished, func(source string) ([]byte, error) {
			answer := a.askSSH(ctx, &sshPrompt{
				title: "解锁旧私钥", address: source, confirmLabel: "解锁并导入",
				detail: "口令仅用于本次导入，不保存到配置或系统钥匙串。",
			})
			if !answer.accepted {
				answer.secret = ""
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, errCatalogMigrationCanceled
			}
			passphrase := []byte(answer.secret)
			answer.secret = ""
			return passphrase, nil
		})
		committed := false
		mygo.RunOnMain(func() {
			if state.sequence != sequence || state.plan != plan || ctx.Err() != nil {
				return
			}
			if err == nil && a.catalogMigrationBlocked() {
				err = errCatalogMigrationBlocked
			}
			if err == nil {
				var published bool
				published, err = createCatalog(plan.destination, prepared.data)
				if published {
					committed = true
					candidate := prepared.candidate
					a.hosts, a.groups, a.keys = candidate.hosts, candidate.groups, candidate.keys
					a.savedWorkspace = candidate.savedWorkspace
					// Imported groups replace the initial catalog, so its UI references
					// cannot remain attached to the newly published group IDs.
					for index := range a.pages {
						page := &a.pages[index]
						page.lastGroup, page.catalogDrag = nil, nil
						page.catalogSlots, page.catalogFlash, page.catalogItems = nil, nil, nil
					}
					a.catalogConfig.loadErr, a.catalogError = nil, nil
					a.catalogConfig.migrated = true
					state.plan, state.unpublished, state.running = nil, nil, false
					a.clearCatalogMigrationPrompts(ctx)
					if err != nil {
						a.files.notice = &uikit.ToastProps{ID: "catalog-migration-cleanup", Title: "会话已导入，临时文件清理失败", Detail: err.Error(), Tone: uikit.ToneDanger}
					} else {
						// The first preference load was blocked by the pending catalog.
						// Re-read it before permitting the existing UUID migration writer.
						a.filePrefs.loadErr = nil
						if prefsErr := a.loadFilePreferences(); prefsErr != nil {
							a.files.notice = &uikit.ToastProps{ID: "catalog-migration-preferences", Title: "会话已导入，文件偏好读取失败", Detail: prefsErr.Error(), Tone: uikit.ToneDanger}
						} else {
							a.files.notice = &uikit.ToastProps{ID: "catalog-migration-complete", Title: "旧会话已导入", Tone: uikit.ToneSuccess}
						}
					}
				}
			}
			a.window.Invalidate()
		})
		if committed {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		cleanupErr := discardMigrationFiles(prepared.created)
		err = errors.Join(err, cleanupErr)
		mygo.RunOnMain(func() {
			if state.sequence != sequence || state.plan != plan {
				return
			}
			state.running = false
			a.clearCatalogMigrationPrompts(ctx)
			state.unpublished = nil
			if cleanupErr != nil {
				state.unpublished = slices.Clone(prepared.created)
			}
			a.catalogConfig.loadErr, a.catalogError = err, err
			a.window.Invalidate()
		})
	})
}

func (a *app) clearCatalogMigrationPrompts(ctx context.Context) {
	a.sshPrompts = slices.DeleteFunc(a.sshPrompts, func(prompt *sshPrompt) bool {
		if prompt.ctx != ctx {
			return false
		}
		prompt.secret, prompt.open = "", false
		return true
	})
}

func prepareCatalogMigration(
	ctx context.Context,
	plan *catalogMigrationPlan,
	unpublished []string,
	unlock func(string) ([]byte, error),
) (preparedCatalogMigration, error) {
	result := preparedCatalogMigration{created: slices.Clone(unpublished)}
	if err := discardMigrationFiles(unpublished); err != nil {
		return result, err
	}
	result.created = []string{}
	var config catalogConfig
	if err := json.Unmarshal(plan.metadata, &config); err != nil {
		return result, err
	}
	imported := map[string]managedKey{}
	for index := range config.Hosts {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		source := plan.keySources[hostID(index)]
		if source == "" {
			continue
		}
		key, exists := imported[source]
		if !exists {
			var created []string
			var err error
			key, created, err = copyLegacyPrivateKey(source, plan.directory, func() ([]byte, error) { return unlock(source) })
			result.created = append(result.created, created...)
			if err != nil {
				return result, fmt.Errorf("导入旧私钥 %s：%w", source, err)
			}
			imported[source] = key
			config.Keys = append(config.Keys, key)
		}
		config.Hosts[index].Session.KeyID = key.ID
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	candidate := &app{}
	if err := candidate.applyStoredCatalog(config, plan.directory); err != nil {
		return result, err
	}
	data, err := candidate.catalogBytes()
	if err != nil {
		return result, err
	}
	result.candidate, result.data = candidate, data
	return result, nil
}
