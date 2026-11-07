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
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type credentialState struct {
	draft                      *credentialDraft
	busy                       bool
	notice                     locales.Text
	serial                     uint64
	cancel                     context.CancelFunc
	agentLoading, agentChecked bool
	agentCount                 int
	agentError                 error
	flashID                    string
	flashAt                    time.Time
}

type credentialDraft struct {
	modal                                           uikit.ModalController
	kind, name, keyType, comment, passphrase, keyID string
	data                                            []byte
	remember                                        bool
	errorText                                       locales.Text
}

type credentialResult struct {
	key        *managedKey
	draft      *credentialDraft
	deletedID  string
	clearedIDs []string
	changes    *store.VaultChanges
	publicKey  string
	notice     locales.Text
	err        error
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

// Crypto runs off the UI thread; only ciphertext and metadata cross into the
// serialized catalog transaction. No external rollback is needed.
func (a *app) credentialWork(work func(context.Context) credentialResult) {
	if a.credentials.busy || !a.businessReady() {
		return
	}
	a.cancelCredentialWork()
	ctx, cancel := context.WithCancel(a.ctx)
	a.credentials.cancel = cancel
	serial := a.credentials.serial
	a.credentials.busy = true
	a.credentials.notice = locales.Text{}
	a.sshWorkers.Go(func() {
		result := work(ctx)
		var stage func()
		stage = func() {
			if ctx.Err() != nil || a.credentials.serial != serial {
				if result.draft != nil {
					clear(result.draft.data)
				}
				return
			}
			if a.catalogSaving {
				a.catalogReady = append(a.catalogReady, stage)
				return
			}
			previous := slices.Clone(a.keys)
			completePasswords := func(error) {}
			if result.err == nil {
				if len(result.clearedIDs) > 0 {
					completePasswords = a.preparePasswordClear(result.clearedIDs)
				}
				if result.key != nil {
					a.keys = append(a.keys, *result.key)
				}
				if result.deletedID != "" {
					a.keys = slices.DeleteFunc(a.keys, func(k managedKey) bool { return k.ID == result.deletedID })
				}
			}
			finish := func(err error) {
				completePasswords(err)
				if err != nil {
					a.keys = previous
					result.err = err
				}
				cancel()
				a.credentials.cancel = nil
				a.credentials.busy = false
				if result.err != nil {
					a.credentials.notice = locales.ErrorText(result.err)
					if a.credentials.draft != nil {
						a.credentials.draft.errorText = locales.ErrorText(result.err)
					}
				} else if result.draft != nil {
					a.credentials.draft = result.draft
				} else {
					if result.key != nil {
						a.credentials.flashID = result.key.ID
						a.credentials.flashAt = time.Time{}
						a.dismissCredentialDraft()
					}
					if result.deletedID != "" || len(result.clearedIDs) > 0 {
						a.dismissCredentialDraft()
					}
					if result.publicKey != "" {
						mygo.Clipboard.WriteText(result.publicKey)
						a.credentials.notice = locales.Deferred(locales.MessagePublicKeyCopied)
					}
					if !result.notice.Empty() {
						a.credentials.notice = result.notice
					}
				}
				a.window.Invalidate()
			}
			if result.err == nil && result.changes != nil {
				a.saveCatalogSecretsAsync(result.changes, finish)
			} else {
				finish(result.err)
			}
		}
		mygo.RunOnMain(stage)
	})
}

func (a *app) clearStoredPasswords(ctx context.Context, ids []string) credentialResult {
	if err := ctx.Err(); err != nil {
		return credentialResult{err: err}
	}
	deleted := make([]string, 0, len(ids))
	for _, id := range ids {
		deleted = append(deleted, sessionCredentialKey(id))
	}
	changes, err := a.vault.Prepare(nil, deleted)
	return credentialResult{clearedIDs: ids, changes: changes, notice: locales.Deferred(
		locales.MessageAllSavedSessionPasswordsCleared,
	), err: err}
}

// Run on the UI thread. Catalog persistence commits before cached credentials
// and runtime flags change; the worker retains secret values until this succeeds.
func (a *app) preparePasswordClear(ids []string) func(error) {
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
	return func(err error) {
		if err != nil {
			for i := range a.hosts {
				profile := &a.hosts[i].session
				if flag, exists := previous[profile.ID]; exists {
					profile.PasswordSaved, profile.PasswordRevision, profile.UpdatedAt = flag.saved, flag.revision, flag.updated
				}
			}
			return
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
	}
}

func (a *app) dismissCredentialDraft() {
	if a.credentials.draft != nil {
		clear(a.credentials.draft.data)
		a.credentials.draft.passphrase = ""
		a.credentials.draft.modal.Close()
	}
}

func (a *app) choosePrivateKey() {
	if a.credentials.busy {
		return
	}
	parent := a.window
	a.credentialWork(func(ctx context.Context) credentialResult {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: locales.MessageImportSshPrivateKey(), ShowHiddenFiles: true})
		if err != nil {
			return credentialResult{err: locales.Errorf(locales.MessageChoosePrivateKey, err)}
		}
		if len(paths) == 0 || ctx.Err() != nil {
			return credentialResult{}
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			return credentialResult{err: locales.Errorf(locales.MessageReadPrivateKey, err)}
		}
		defer clear(data)
		raw, err := ssh.ParseRawPrivateKey(data)
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return credentialResult{draft: &credentialDraft{kind: "import", name: filepath.Base(paths[0]), data: slices.Clone(data), remember: true}}
		}
		if err != nil {
			return credentialResult{err: locales.Errorf(locales.MessageParsePrivateKey, err)}
		}
		return a.storeManagedKey(ctx, data, raw, filepath.Base(paths[0]), "", "", false)
	})
}

func (a *app) beginKeyGeneration() {
	if a.credentials.busy {
		return
	}
	current, err := user.Current()
	if err != nil {
		a.credentials.notice = locales.Concat(locales.Deferred(locales.MessageReadCurrentUsername), locales.ErrorText(err))
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		a.credentials.notice = locales.Concat(locales.Deferred(locales.MessageReadLocalHostname), locales.ErrorText(err))
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
		d.errorText = locales.Deferred(locales.MessageEnterAKeyName)
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
				return credentialResult{err: locales.Errorf(locales.MessageIncorrectKeyPassphraseOrUnsupportedFormat, err)}
			}
		case "generate":
			if snapshot.keyType == "RSA 4096" {
				raw, err = rsa.GenerateKey(rand.Reader, 4096)
			} else {
				_, raw, err = ed25519.GenerateKey(rand.Reader)
			}
			if err != nil {
				return credentialResult{err: locales.Errorf(locales.MessageGeneratePrivateKey, err)}
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
				return credentialResult{err: locales.Errorf(locales.MessageEncodePrivateKey, err)}
			}
			data = pem.EncodeToMemory(block)
			defer clear(data)
		default:
			return credentialResult{err: locales.NewError(locales.MessageUnknownKeyOperation)}
		}
		return a.storeManagedKey(ctx, data, raw, snapshot.name, snapshot.comment, snapshot.passphrase, snapshot.remember)
	})
}

func (a *app) storeManagedKey(ctx context.Context, data []byte, raw any, name, comment, passphrase string, remember bool) credentialResult {
	if err := ctx.Err(); err != nil {
		return credentialResult{err: err}
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return credentialResult{err: locales.Errorf(locales.MessageReadPublicKey, err)}
	}
	id := newSessionID()
	public := ssh.MarshalAuthorizedKey(signer.PublicKey())
	if comment != "" {
		public = []byte(strings.TrimSpace(string(public)) + " " + comment + "\n")
	}
	items := []vault.PlainItem{{ID: privateCredentialKey(id), Kind: "private_key", Revision: id, Data: data}}
	if remember && passphrase != "" {
		phrase := []byte(passphrase)
		defer clear(phrase)
		items = append(items, vault.PlainItem{ID: "key/" + id + "/passphrase", Kind: "key_passphrase", Revision: id, Data: phrase})
	}
	changes, err := a.vault.Prepare(items, nil)
	if err != nil {
		return credentialResult{err: err}
	}
	keyType, bits := managedKeyType(raw, signer.PublicKey())
	key := managedKey{ID: id, SecretID: privateCredentialKey(id), PublicKey: public, Name: name, Type: keyType, Bits: bits, Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), Comment: comment, CreatedAt: time.Now().UTC()}
	return credentialResult{key: &key, changes: changes, notice: locales.Deferred(
		locales.MessageKeySavedYouCanSelectItInTheSessionAuthenticationOptions,
	)}
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
	if a.gatewayPool.keyInUse(id) {
		references["gateway/"+id] = true
	}
	add := func(config sessionConfig, capturedKey string) {
		if config.KeyID == id || capturedKey == id {
			references[config.ID] = true
		}
		if config.GatewayRoute != nil {
			for _, hop := range *config.GatewayRoute {
				if hop.SSH != nil && hop.SSH.KeyID == id {
					references[hop.SSH.ID] = true
				}
			}
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
	for _, closed := range a.closedTabs {
		if closed.section == sectionFiles {
			fileSessions[closed.file] = true
			continue
		}
		add(closed.endpoint.session, "")
		if closed.connection != nil {
			groups[closed.connection] = true
		}
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
		// Live and recoverable views keep their captured jump-host plans after edits.
		for _, step := range group.plan.steps {
			add(step.endpoint.session, step.key.ID)
		}
	}
	return len(references)
}

func (a *app) copyManagedPublicKey(key managedKey) {
	a.credentialWork(func(ctx context.Context) credentialResult {
		if ctx.Err() != nil {
			return credentialResult{err: ctx.Err()}
		}
		return credentialResult{publicKey: string(key.PublicKey)}
	})
}

// agentState queries the platform's local agent with bounded I/O.
func agentState() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := sshclient.DialAgent(ctx)
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
