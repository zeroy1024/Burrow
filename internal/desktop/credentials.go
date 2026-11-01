package desktop

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"sshdesk/internal/uikit"
)

type credentialState struct {
	draft                      *credentialDraft
	busy                       bool
	notice                     string
	serial                     uint64
	cancel                     context.CancelFunc
	agentLoading, agentChecked bool
	agentCount                 int
	agentError                 error
	flashID                    string
	flashAt                    time.Time
}

type credentialDraft struct {
	kind, name, keyType, comment, passphrase, keyID string
	data                                            []byte
	remember                                        bool
	errorText                                       string
}

type credentialResult struct {
	key          *managedKey
	draft        *credentialDraft
	deletedID    string
	restoreKey   *managedKey
	restoreIndex int
	clearedIDs   []string
	passwords    *passwordClearSnapshot
	publicKey    string
	notice       string
	err          error
}

func (a *app) openCredentialSettings() { a.openSettingsCategory(5) }

func (a *app) cancelCredentialWork() {
	if a.credentials.cancel != nil {
		a.credentials.cancel()
		a.credentials.cancel = nil
	}
	a.credentials.serial++
	a.credentials.busy = false
}

// A worker owns file bytes and crypto operations; only its result touches UI state.
func (a *app) credentialWork(work func(context.Context) credentialResult) {
	if a.credentials.busy {
		return
	}
	a.cancelCredentialWork()
	ctx, cancel := context.WithCancel(a.ctx)
	a.credentials.cancel = cancel
	serial := a.credentials.serial
	a.credentials.busy = true
	a.credentials.notice = ""
	a.sshWorkers.Go(func() {
		result := work(ctx)
		accepted := false
		discardNewKey := false
		passwordRollback := false
		mygo.RunOnMain(func() {
			if ctx.Err() != nil || a.credentials.serial != serial {
				return
			}
			accepted = true
			cancel()
			a.credentials.cancel = nil
			a.credentials.busy = false
			if result.passwords != nil {
				if err := a.commitPasswordClear(result.clearedIDs); err != nil {
					result.err = errors.Join(result.err, err)
					passwordRollback = true
				}
			}
			catalogChanged := result.key != nil || result.restoreKey != nil
			if result.key != nil {
				a.keys = append(a.keys, *result.key)
			}
			if result.restoreKey != nil {
				a.keys = slices.Insert(a.keys, min(result.restoreIndex, len(a.keys)), *result.restoreKey)
			}
			if catalogChanged {
				if err := a.saveCatalog(); err != nil {
					result.err = errors.Join(result.err, fmt.Errorf("保存凭据目录：%w", err))
					if result.key != nil {
						a.keys = slices.DeleteFunc(a.keys, func(key managedKey) bool { return key.ID == result.key.ID })
						discardNewKey = true
					}
				}
			}
			if result.err != nil {
				a.credentials.notice = result.err.Error()
				if a.credentials.draft != nil {
					a.credentials.draft.errorText = result.err.Error()
				}
			} else if result.draft != nil {
				a.credentials.draft = result.draft
			} else {
				if result.key != nil {
					a.credentials.flashID = result.key.ID
					a.credentials.flashAt = time.Time{}
					a.credentials.notice = "密钥已保存，可从会话认证列表中选择。"
					a.dismissCredentialDraft()
				}
				if result.deletedID != "" || len(result.clearedIDs) > 0 {
					a.dismissCredentialDraft()
				}
				if result.publicKey != "" {
					mygo.Clipboard.WriteText(result.publicKey)
					a.credentials.notice = "公钥已复制"
				}
				if result.notice != "" {
					a.credentials.notice = result.notice
				}
			}
			if a.window != nil {
				a.window.Invalidate()
			}
			if discardNewKey || passwordRollback {
				a.credentials.busy = true
			}
		})
		if result.passwords != nil {
			var rollbackErr error
			if !accepted || passwordRollback {
				rollbackErr = result.passwords.restore()
			}
			result.passwords.release()
			if passwordRollback || rollbackErr != nil {
				mygo.RunOnMain(func() {
					err := errors.Join(result.err, rollbackErr)
					if a.credentials.serial == serial {
						a.credentials.busy = false
						a.credentials.notice = err.Error()
						if a.credentials.draft != nil {
							a.credentials.draft.errorText = err.Error()
						}
					} else {
						a.files.notice = &uikit.ToastProps{ID: "password-clear-rollback", Tone: uikit.ToneDanger, Title: "恢复保存的密码失败", Detail: err.Error()}
					}
					if a.window != nil {
						a.window.Invalidate()
					}
				})
			}
		}
		if !accepted || discardNewKey {
			if result.draft != nil {
				clear(result.draft.data)
			}
			if result.key != nil {
				err := discardManagedKey(result.key.ID, []string{result.key.Path, result.key.Path + ".pub"})
				mygo.RunOnMain(func() {
					if a.credentials.serial != serial {
						return
					}
					a.credentials.busy = false
					if err != nil {
						a.credentials.notice += "\n清理未保存的密钥失败：" + err.Error()
					}
					if a.window != nil {
						a.window.Invalidate()
					}
				})
			}
		}
	})
}

type passwordClearSnapshot struct {
	ids       []string
	values    map[string][]byte
	attempted []string
}

func (s *passwordClearSnapshot) restore() error {
	var result error
	for _, key := range s.attempted {
		if err := mygo.Secrets.Set(key, s.values[key]); err != nil {
			result = errors.Join(result, fmt.Errorf("恢复保存的会话密码：%w", err))
		}
	}
	return result
}

func (s *passwordClearSnapshot) release() {
	for _, value := range s.values {
		clear(value)
	}
	s.values = nil
}

func clearStoredPasswords(ctx context.Context, ids []string) credentialResult {
	snapshot := &passwordClearSnapshot{ids: slices.Clone(ids), values: map[string][]byte{}}
	fail := func(err error) credentialResult {
		err = errors.Join(err, snapshot.restore())
		snapshot.release()
		return credentialResult{err: err}
	}
	for _, id := range snapshot.ids {
		for _, key := range []string{sessionCredentialKey(id), sessionLegacyPasswordKey(id)} {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			value, err := mygo.Secrets.Get(key)
			if errors.Is(err, mygo.ErrSecretNotFound) {
				continue
			}
			if err != nil {
				return fail(fmt.Errorf("读取原有会话密码：%w", err))
			}
			snapshot.values[key] = value
		}
	}
	for _, id := range snapshot.ids {
		for _, key := range []string{sessionCredentialKey(id), sessionLegacyPasswordKey(id)} {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			if _, present := snapshot.values[key]; !present {
				continue
			}
			snapshot.attempted = append(snapshot.attempted, key)
			if err := mygo.Secrets.Delete(key); err != nil {
				return fail(fmt.Errorf("清除保存的密码：%w", err))
			}
		}
	}
	return credentialResult{clearedIDs: snapshot.ids, passwords: snapshot, notice: "已清除所有保存的会话密码"}
}

// Run on the UI thread. Catalog persistence commits before cached credentials
// and runtime flags change; the worker retains secret values until this succeeds.
func (a *app) commitPasswordClear(ids []string) error {
	type previousFlag struct {
		saved    bool
		revision string
		updated  time.Time
	}
	previous := map[string]previousFlag{}
	changed := map[string]bool{}
	for _, id := range ids {
		changed[id] = true
	}
	updated := time.Now().UTC()
	for i := range a.hosts {
		profile := &a.hosts[i].session
		if changed[profile.ID] {
			previous[profile.ID] = previousFlag{saved: profile.PasswordSaved, revision: profile.PasswordRevision, updated: profile.UpdatedAt}
			profile.PasswordSaved, profile.PasswordRevision, profile.UpdatedAt = false, "", updated
		}
	}
	if err := a.saveCatalog(); err != nil {
		for i := range a.hosts {
			profile := &a.hosts[i].session
			if flag, exists := previous[profile.ID]; exists {
				profile.PasswordSaved, profile.PasswordRevision, profile.UpdatedAt = flag.saved, flag.revision, flag.updated
			}
		}
		return fmt.Errorf("保存密码清理状态：%w", err)
	}
	clearFlag := func(config *sessionConfig) {
		if changed[config.ID] {
			config.PasswordSaved, config.PasswordRevision, config.UpdatedAt = false, "", updated
		}
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			clearFlag(&session.config)
			clearFlag(&session.endpoint.session)
		}
	}
	files := map[*fileSession]bool{}
	for _, session := range a.fileSessions {
		files[session] = true
	}
	for _, task := range a.transfers.tasks {
		files[task.session] = true
	}
	for session := range files {
		if changed[session.config.ID] || changed[session.endpoint.session.ID] {
			session.ftpSecret = ""
		}
		clearFlag(&session.config)
		clearFlag(&session.endpoint.session)
	}
	return nil
}

func (a *app) dismissCredentialDraft() {
	if a.credentials.draft != nil {
		clear(a.credentials.draft.data)
		a.credentials.draft.passphrase = ""
	}
	a.credentials.draft = nil
}

func (a *app) choosePrivateKey() {
	if a.credentials.busy {
		return
	}
	parent := a.window
	a.credentialWork(func(ctx context.Context) credentialResult {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "导入 SSH 私钥", ShowHiddenFiles: true})
		if err != nil {
			return credentialResult{err: fmt.Errorf("选择私钥：%w", err)}
		}
		if len(paths) == 0 || ctx.Err() != nil {
			return credentialResult{}
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			return credentialResult{err: fmt.Errorf("读取私钥：%w", err)}
		}
		defer clear(data)
		raw, err := ssh.ParseRawPrivateKey(data)
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return credentialResult{draft: &credentialDraft{kind: "import", name: filepath.Base(paths[0]), data: slices.Clone(data), remember: true}}
		}
		if err != nil {
			return credentialResult{err: fmt.Errorf("解析私钥：%w", err)}
		}
		return storeManagedKey(ctx, data, raw, filepath.Base(paths[0]), "", "", false)
	})
}

func (a *app) beginKeyGeneration() {
	if a.credentials.busy {
		return
	}
	current, err := user.Current()
	if err != nil {
		a.credentials.notice = "读取当前用户名：" + err.Error()
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		a.credentials.notice = "读取本机名称：" + err.Error()
		return
	}
	a.credentials.draft = &credentialDraft{kind: "generate", keyType: "Ed25519", comment: current.Username + "@" + hostname, remember: true}
}

func (a *app) submitCredentialDraft() {
	d := a.credentials.draft
	if d == nil || a.credentials.busy {
		return
	}
	name := strings.TrimSpace(d.name)
	if name == "" {
		d.errorText = "请填写密钥名称"
		return
	}
	snapshot := *d
	snapshot.name = name
	snapshot.data = slices.Clone(d.data)
	a.credentialWork(func(ctx context.Context) credentialResult {
		defer clear(snapshot.data)
		var raw any
		var err error
		data := snapshot.data
		switch snapshot.kind {
		case "import":
			raw, err = ssh.ParseRawPrivateKeyWithPassphrase(data, []byte(snapshot.passphrase))
			if err != nil {
				return credentialResult{err: fmt.Errorf("私钥口令错误或格式不支持：%w", err)}
			}
		case "generate":
			if snapshot.keyType == "RSA 4096" {
				raw, err = rsa.GenerateKey(rand.Reader, 4096)
			} else {
				_, raw, err = ed25519.GenerateKey(rand.Reader)
			}
			if err != nil {
				return credentialResult{err: fmt.Errorf("生成私钥：%w", err)}
			}
			if ctx.Err() != nil {
				return credentialResult{err: ctx.Err()}
			}
			var block *pem.Block
			if snapshot.passphrase != "" {
				block, err = ssh.MarshalPrivateKeyWithPassphrase(raw, snapshot.comment, []byte(snapshot.passphrase))
			} else {
				block, err = ssh.MarshalPrivateKey(raw, snapshot.comment)
			}
			if err != nil {
				return credentialResult{err: fmt.Errorf("编码私钥：%w", err)}
			}
			data = pem.EncodeToMemory(block)
			defer clear(data)
		default:
			return credentialResult{err: errors.New("未知密钥操作")}
		}
		return storeManagedKey(ctx, data, raw, snapshot.name, snapshot.comment, snapshot.passphrase, snapshot.remember)
	})
}

func storeManagedKey(ctx context.Context, data []byte, raw any, name, comment, passphrase string, remember bool) (result credentialResult) {
	if err := ctx.Err(); err != nil {
		return credentialResult{err: err}
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return credentialResult{err: fmt.Errorf("读取公钥：%w", err)}
	}
	directory, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return credentialResult{err: err}
	}
	directory = filepath.Join(directory, "keys")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return credentialResult{err: fmt.Errorf("创建密钥目录：%w", err)}
	}
	id := newSessionID()
	destination := filepath.Join(directory, id)
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return credentialResult{err: fmt.Errorf("创建密钥文件：%w", err)}
	}
	created := []string{destination}
	defer func() {
		if result.key == nil {
			result.err = errors.Join(result.err, discardManagedKey(id, created))
		}
	}()
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		return credentialResult{err: fmt.Errorf("写入密钥文件：%w", err)}
	}
	public := ssh.MarshalAuthorizedKey(signer.PublicKey())
	if comment != "" {
		public = []byte(strings.TrimSpace(string(public)) + " " + comment + "\n")
	}
	publicFile, err := os.OpenFile(destination+".pub", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return credentialResult{err: fmt.Errorf("创建公钥文件：%w", err)}
	}
	created = append(created, destination+".pub")
	_, writeErr = publicFile.Write(public)
	if err := errors.Join(writeErr, publicFile.Close()); err != nil {
		return credentialResult{err: fmt.Errorf("写入公钥：%w", err)}
	}
	if remember && passphrase != "" {
		if err := mygo.Secrets.Set("key/"+id+"/passphrase", []byte(passphrase)); err != nil {
			return credentialResult{err: fmt.Errorf("保存私钥口令：%w", err)}
		}
	}
	if err := ctx.Err(); err != nil {
		return credentialResult{err: err}
	}
	keyType, bits := managedKeyType(raw, signer.PublicKey())
	key := managedKey{ID: id, Name: name, Type: keyType, Bits: bits, Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), Path: destination, Comment: comment, CreatedAt: time.Now().UTC()}
	return credentialResult{key: &key}
}

// Only files created by the current, unpublished key operation are rolled back.
func discardManagedKey(id string, paths []string) error {
	var cleanup error
	if err := mygo.Secrets.Delete("key/" + id + "/passphrase"); err != nil {
		cleanup = fmt.Errorf("清理口令：%w", err)
	}
	for _, path := range paths {
		if err := mygo.Shell.TrashItem(path); err != nil {
			cleanup = errors.Join(cleanup, fmt.Errorf("清理密钥文件：%w", err))
		}
	}
	return cleanup
}

func managedKeyType(raw any, pub ssh.PublicKey) (string, int) {
	if signer, ok := raw.(crypto.Signer); ok {
		switch public := signer.Public().(type) {
		case *rsa.PublicKey:
			return "RSA", public.N.BitLen()
		case ed25519.PublicKey:
			return "Ed25519", 256
		case *ecdsa.PublicKey:
			return "ECDSA", public.Curve.Params().BitSize
		}
	}
	return pub.Type(), 0
}

func (a *app) keyUseCount(id string) int {
	references := map[string]bool{}
	add := func(config sessionConfig, capturedKey string) {
		if config.KeyID == id || capturedKey == id {
			references[config.ID] = true
		}
	}
	for _, h := range a.hosts {
		add(h.session, "")
	}
	groups := map[*sshConnectionGroup]bool{}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			add(session.endpoint.session, "")
			add(session.config, "")
			if session.connection != nil {
				groups[session.connection] = true
			}
		}
	}
	fileSessions := map[*fileSession]bool{}
	for _, session := range a.fileSessions {
		fileSessions[session] = true
	}
	for _, task := range a.transfers.tasks {
		fileSessions[task.session] = true
	}
	for session := range fileSessions {
		add(session.endpoint.session, "")
		add(session.config, "")
		if session.connection != nil {
			groups[session.connection] = true
		}
	}
	for group := range groups {
		add(group.endpoint.session, "")
		// A running group's jump hosts remain immutable after catalog edits.
		// Their captured keys must survive until all consumers release the plan.
		for _, step := range group.plan.steps {
			add(step.endpoint.session, step.key.ID)
		}
	}
	return len(references)
}

// Metadata has already committed on the UI thread. Keep both key files in a
// recoverable staging directory until removing the credential and trashing the
// directory succeeds; failures restore the original file paths and credential.
func trashManagedKey(ctx context.Context, key managedKey, index int) credentialResult {
	secretName := "key/" + key.ID + "/passphrase"
	secret, err := mygo.Secrets.Get(secretName)
	secretExists := err == nil
	if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
		return credentialResult{restoreKey: &key, restoreIndex: index, err: fmt.Errorf("读取保存的口令：%w", err)}
	}
	defer clear(secret)
	if err := ctx.Err(); err != nil {
		return credentialResult{restoreKey: &key, restoreIndex: index, err: err}
	}
	directory, err := os.MkdirTemp(filepath.Dir(key.Path), ".delete-key-*")
	if err != nil {
		return credentialResult{restoreKey: &key, restoreIndex: index, err: fmt.Errorf("准备密钥删除：%w", err)}
	}
	type movedFile struct{ source, destination string }
	moved := []movedFile{}
	secretDeleted := false
	rollback := func(cause error) credentialResult {
		var rollbackErr error
		privateRestored := len(moved) == 0
		for i := len(moved) - 1; i >= 0; i-- {
			item := moved[i]
			if err := os.Rename(item.destination, item.source); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("恢复密钥文件（保留在 %s）：%w", item.destination, err))
			} else if item.source == key.Path {
				privateRestored = true
			}
		}
		if secretDeleted {
			if err := mygo.Secrets.Set(secretName, secret); err != nil {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("恢复保存的口令：%w", err))
			}
		}
		if rollbackErr == nil {
			if err := os.Remove(directory); err != nil {
				rollbackErr = fmt.Errorf("清理删除暂存目录：%w", err)
			}
		}
		result := credentialResult{err: errors.Join(cause, rollbackErr)}
		if privateRestored {
			result.restoreKey, result.restoreIndex = &key, index
		} else {
			result.deletedID = key.ID
		}
		return result
	}
	for _, source := range []string{key.Path, key.Path + ".pub"} {
		if source != key.Path {
			if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return rollback(err)
			}
		}
		destination := filepath.Join(directory, filepath.Base(source))
		if err := os.Rename(source, destination); err != nil {
			return rollback(fmt.Errorf("暂存密钥文件：%w", err))
		}
		moved = append(moved, movedFile{source: source, destination: destination})
	}
	if secretExists {
		// A failed credential API call may already have changed the item. Restore
		// its captured value on every failure after attempting deletion.
		secretDeleted = true
		if err := mygo.Secrets.Delete(secretName); err != nil {
			return rollback(fmt.Errorf("删除保存的口令：%w", err))
		}
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := mygo.Shell.TrashItem(directory); err != nil {
		return rollback(fmt.Errorf("移动私钥到废纸篓：%w", err))
	}
	return credentialResult{deletedID: key.ID, notice: "密钥已移到废纸篓"}
}

func (a *app) copyManagedPublicKey(key managedKey) {
	a.credentialWork(func(ctx context.Context) credentialResult {
		data, err := os.ReadFile(key.Path + ".pub")
		if err != nil {
			return credentialResult{err: fmt.Errorf("读取公钥：%w", err)}
		}
		if _, _, _, _, err := ssh.ParseAuthorizedKey(data); err != nil {
			return credentialResult{err: fmt.Errorf("解析公钥：%w", err)}
		}
		if ctx.Err() != nil {
			return credentialResult{err: ctx.Err()}
		}
		return credentialResult{publicKey: string(data)}
	})
}

// agentState only queries the local Unix socket, with bounded I/O.
func agentState() (int, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return 0, errors.New("未检测到 ssh-agent（SSH_AUTH_SOCK）")
	}
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return 0, err
	}
	keys, err := agent.NewClient(conn).List()
	return len(keys), err
}

func (a *app) refreshCredentialAgent() {
	if a.credentials.agentLoading {
		return
	}
	a.credentials.agentLoading = true
	a.sshWorkers.Go(func() {
		count, err := agentState()
		mygo.RunOnMain(func() {
			a.credentials.agentLoading = false
			a.credentials.agentChecked = true
			a.credentials.agentCount, a.credentials.agentError = count, err
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	})
}
