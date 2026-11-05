// Package vault encrypts local credentials and owns their in-memory key lifetime.
package vault

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"sshdesk/internal/localfs"
	"sshdesk/internal/store"
)

var (
	ErrLocked   = errors.New("凭据库尚未解锁")
	ErrNotFound = errors.New("凭据不存在")
	ErrUnlock   = errors.New("无法解锁，请检查密码或凭据数据")
)

type PlainItem struct {
	ID, Kind, Revision string
	Data               []byte
}

// KeyStore holds only automatic-unlock wrapping keys. Credential ciphertext and
// application-password derivation remain independent of the platform store.
type KeyStore interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

type KeyCleanup struct {
	File    string
	store   KeyStore
	account string
}

func (k KeyCleanup) Run(recycle func(string) error) error {
	if k.File != "" {
		return recycle(k.File)
	}
	if k.store != nil {
		return k.store.Delete(k.account)
	}
	return nil
}

type Manager struct {
	mu        sync.RWMutex
	db        *store.Store
	directory string
	snapshot  store.VaultSnapshot
	key       []byte
	blocked   bool
	keyStore  KeyStore
}

func Open(ctx context.Context, db *store.Store, directory string) (*Manager, error) {
	return OpenWithKeyStore(ctx, db, directory, nil)
}

func OpenWithKeyStore(ctx context.Context, db *store.Store, directory string, keys KeyStore) (*Manager, error) {
	snapshot, err := db.ReadVault(ctx)
	if err != nil {
		return nil, err
	}
	m := &Manager{db: db, directory: directory, snapshot: snapshot, keyStore: keys}
	switch snapshot.State {
	case "pending_init", "pending_migration":
		if !reflect.DeepEqual(snapshot.Header, store.VaultHeader{}) || len(snapshot.Items) != 0 {
			return nil, errors.New("invalid pending vault")
		}
	case "ready":
		if err := validateHeader(snapshot.Header); err != nil {
			return nil, err
		}
		for _, item := range snapshot.Items {
			if err := validateItem(snapshot.Header, item); err != nil {
				return nil, err
			}
		}
		if snapshot.Header.Mode == "local" {
			kek, err := m.readLocalKey(snapshot.Header)
			if err != nil {
				return nil, err
			}
			defer clear(kek)
			m.key, err = unwrap(snapshot.Header, kek)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, errors.New("unsupported vault state")
	}
	// Retain the header only. Secret ciphertext is read when requested.
	m.snapshot.Items = nil
	if keys != nil && snapshot.State == "ready" && snapshot.Header.Mode == "local" && snapshot.Header.KeyStorage != "keychain" {
		// This is an explicit file-to-Keychain upgrade, never a read-error fallback.
		// Rotate the DEK and retain the old file as a migration/backup source.
		if _, err := m.ChangeProtection(ctx, "", "", false); err != nil {
			m.Close()
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) State() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.blocked {
		return "blocked"
	}
	return m.snapshot.State
}
func (m *Manager) RequiresPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshot.Header.Mode == "password"
}
func (m *Manager) Unlocked() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.key) == 32
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.key)
	m.key = nil
}

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < 6 || length > 64 {
		return errors.New("解锁密码长度须为 6–64 个字符")
	}
	return nil
}

func (m *Manager) Unlock(ctx context.Context, password []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.blocked || m.snapshot.State != "ready" || m.snapshot.Header.Mode != "password" {
		return errors.New("vault does not require a password")
	}
	if len(password) > 1024 {
		return ErrUnlock
	}
	kek := derive(password, m.snapshot.Header)
	defer clear(kek)
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := unwrap(m.snapshot.Header, kek)
	if err != nil {
		return err
	}
	clear(m.key)
	m.key = key
	return nil
}

func (m *Manager) Get(ctx context.Context, id string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.key) != 32 {
		return nil, ErrLocked
	}
	item, err := m.db.ReadVaultItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decrypt(m.snapshot.Header, m.key, item)
}

func (m *Manager) Prepare(items []PlainItem, deleted []string) (*store.VaultChanges, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.key) != 32 {
		return nil, ErrLocked
	}
	changes := &store.VaultChanges{Epoch: m.snapshot.Header.Epoch, Delete: append([]string{}, deleted...)}
	for _, plain := range items {
		item, err := encrypt(m.snapshot.Header, m.key, plain)
		if err != nil {
			return nil, err
		}
		changes.Put = append(changes.Put, item)
	}
	return changes, nil
}

// Initialize is only legal for an explicit pending marker created by the schema
// upgrade. A missing header or local key never silently creates a replacement.
func (m *Manager) Initialize(ctx context.Context, items []PlainItem, keys map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.blocked || (m.snapshot.State != "pending_init" && m.snapshot.State != "pending_migration") {
		return errors.New("vault already initialized")
	}
	next, key, err := m.prepareHeader("", false)
	if err != nil {
		return err
	}
	defer clear(key)
	for _, plain := range items {
		item, err := encrypt(next.Header, key, plain)
		if err != nil {
			return err
		}
		next.Items = append(next.Items, item)
	}
	return m.commit(ctx, next, key, keys)
}

// ChangeProtection rotates the DEK as well as its wrapping key, so old wrapping
// material cannot decrypt the current generation. Old backups remain readable.
func (m *Manager) ChangeProtection(ctx context.Context, current, nextPassword string, requirePassword bool) (KeyCleanup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return KeyCleanup{}, err
	}
	if len(m.key) != 32 {
		return KeyCleanup{}, ErrLocked
	}
	old := m.snapshot.Header
	if old.Mode == "password" {
		if len(current) > 1024 {
			return KeyCleanup{}, ErrUnlock
		}
		input := []byte(current)
		kek := derive(input, old)
		clear(input)
		verified, err := unwrap(old, kek)
		clear(kek)
		if err != nil {
			return KeyCleanup{}, err
		}
		clear(verified)
	}
	if requirePassword {
		if err := ValidatePassword(nextPassword); err != nil {
			return KeyCleanup{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return KeyCleanup{}, err
	}
	saved, err := m.db.ReadVault(ctx)
	if err != nil {
		return KeyCleanup{}, err
	}
	if !reflect.DeepEqual(saved.Header, old) || saved.State != "ready" {
		return KeyCleanup{}, errors.New("vault changed before protection update")
	}
	next, key, err := m.prepareHeader(nextPassword, requirePassword)
	if err != nil {
		return KeyCleanup{}, err
	}
	defer clear(key)
	for _, item := range saved.Items {
		data, err := decrypt(old, m.key, item)
		if err != nil {
			return KeyCleanup{}, err
		}
		encrypted, err := encrypt(next.Header, key, PlainItem{ID: item.ID, Kind: item.Kind, Revision: item.Revision, Data: data})
		clear(data)
		if err != nil {
			return KeyCleanup{}, err
		}
		next.Items = append(next.Items, encrypted)
	}
	if err := m.commit(ctx, next, key, nil); err != nil {
		return KeyCleanup{}, err
	}
	if old.Mode == "local" {
		if old.KeyStorage == "keychain" {
			return KeyCleanup{store: m.keyStore, account: keychainAccount(old)}, nil
		}
		return KeyCleanup{File: m.localPath(old.LocalKeyID)}, nil
	}
	return KeyCleanup{}, nil
}

func (m *Manager) prepareHeader(password string, protected bool) (store.VaultSnapshot, []byte, error) {
	h := store.VaultHeader{Version: 2, ID: m.snapshot.Header.ID, Epoch: m.snapshot.Header.Epoch + 1, Mode: "local"}
	if h.ID == "" {
		h.ID = randomID()
	}
	key := randomBytes(32)
	var kek []byte
	if protected {
		h.Mode = "password"
		h.Salt = randomBytes(16)
		h.Memory, h.Iterations, h.Parallelism = 64*1024, 3, 4
		input := []byte(password)
		kek = derive(input, h)
		clear(input)
	} else {
		h.LocalKeyID = randomID()
		h.KeyStorage = "file"
		if m.keyStore != nil {
			h.KeyStorage = "keychain"
		}
		kek = randomBytes(32)
		if err := m.writeLocalKey(h, kek); err != nil {
			clear(kek)
			clear(key)
			return store.VaultSnapshot{}, nil, err
		}
	}
	defer clear(kek)
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		clear(key)
		return store.VaultSnapshot{}, nil, err
	}
	h.Nonce = randomBytes(aead.NonceSize())
	h.WrappedKey = aead.Seal(nil, h.Nonce, key, headerAAD(h))
	return store.VaultSnapshot{State: "ready", Header: h}, key, nil
}

func (m *Manager) commit(ctx context.Context, next store.VaultSnapshot, key []byte, keys map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Once committing starts, finish or reconcile the durable outcome even when
	// the window closes. This never starts user/network work after cancellation.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	err := m.db.ReplaceVault(finish, m.snapshot, next, keys)
	observed, readErr := m.db.ReadVault(finish)
	if readErr != nil {
		m.invalidate()
		return errors.Join(err, fmt.Errorf("无法确认凭据库提交结果：%w", readErr))
	}
	if observed.State == "ready" && reflect.DeepEqual(observed.Header, next.Header) {
		if len(observed.Items) != len(next.Items) {
			m.invalidate()
			return errors.New("committed vault item count mismatch")
		}
		for _, item := range observed.Items {
			plain, verifyErr := decrypt(next.Header, key, item)
			clear(plain)
			if verifyErr != nil {
				m.invalidate()
				return verifyErr
			}
		}
		clear(m.key)
		m.key = bytes.Clone(key)
		next.Items = nil
		m.snapshot = next
		return nil
	}
	if !reflect.DeepEqual(observed.Header, m.snapshot.Header) || observed.State != m.snapshot.State {
		m.invalidate()
		return errors.Join(err, errors.New("凭据库提交状态不一致"))
	}
	if err == nil {
		return errors.New("vault replacement was not committed")
	}
	return err
}

// Called with the write lock held when the durable key generation cannot be
// verified. Reusing the old header after this point would re-enable stale keys.
func (m *Manager) invalidate() {
	clear(m.key)
	m.key = nil
	m.blocked = true
}

func validateHeader(h store.VaultHeader) error {
	if (h.Version != 1 && h.Version != 2) || !validID(h.ID) || h.Epoch < 1 || len(h.Nonce) != 24 || len(h.WrappedKey) != 48 {
		return errors.New("unsupported or invalid vault header")
	}
	if h.Version == 1 && h.KeyStorage != "" {
		return errors.New("invalid legacy key storage")
	}
	switch h.Mode {
	case "local":
		if h.Version == 2 && h.KeyStorage != "file" && h.KeyStorage != "keychain" {
			return errors.New("unsupported automatic key storage")
		}
		if !validID(h.LocalKeyID) || len(h.Salt) != 0 || h.Memory != 0 || h.Iterations != 0 || h.Parallelism != 0 {
			return errors.New("invalid local vault header")
		}
	case "password":
		if h.KeyStorage != "" || h.LocalKeyID != "" || len(h.Salt) != 16 || h.Memory != 64*1024 || h.Iterations != 3 || h.Parallelism != 4 {
			return errors.New("unsupported password derivation parameters")
		}
	default:
		return errors.New("unsupported vault protection")
	}
	return nil
}

func expectedKind(id string) string {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[1] == "" {
		return ""
	}
	switch {
	case parts[0] == "session" && parts[2] == "credential":
		return "session_password"
	case parts[0] == "key" && parts[2] == "private":
		return "private_key"
	case parts[0] == "key" && parts[2] == "passphrase":
		return "key_passphrase"
	default:
		return ""
	}
}
func validateItem(h store.VaultHeader, item store.VaultItem) error {
	if expectedKind(item.ID) == "" || expectedKind(item.ID) != item.Kind || item.Revision == "" || item.Epoch != h.Epoch || len(item.Nonce) != 24 || len(item.Ciphertext) < 16 || len(item.Ciphertext) > 16*1024*1024 {
		return errors.New("invalid encrypted credential record")
	}
	return nil
}
func encrypt(h store.VaultHeader, key []byte, plain PlainItem) (store.VaultItem, error) {
	item := store.VaultItem{ID: plain.ID, Kind: plain.Kind, Revision: plain.Revision, Epoch: h.Epoch, Nonce: randomBytes(24)}
	if len(plain.Data) > 16*1024*1024-16 {
		return item, errors.New("credential is too large")
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return item, err
	}
	item.Ciphertext = aead.Seal(nil, item.Nonce, plain.Data, itemAAD(h, item))
	return item, validateItem(h, item)
}
func decrypt(h store.VaultHeader, key []byte, item store.VaultItem) ([]byte, error) {
	if err := validateItem(h, item); err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	data, err := aead.Open(nil, item.Nonce, item.Ciphertext, itemAAD(h, item))
	if err != nil {
		return nil, fmt.Errorf("凭据完整性校验失败：%w", err)
	}
	return data, nil
}
func derive(password []byte, h store.VaultHeader) []byte {
	return argon2.IDKey(password, h.Salt, h.Iterations, h.Memory, h.Parallelism, 32)
}
func unwrap(h store.VaultHeader, kek []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, err
	}
	key, err := aead.Open(nil, h.Nonce, h.WrappedKey, headerAAD(h))
	if err != nil {
		return nil, ErrUnlock
	}
	return key, nil
}

// JSON arrays give the AAD an unambiguous field order and length encoding.
func headerAAD(h store.VaultHeader) []byte {
	fields := []any{"burrow.vault.wrap", h.Version, h.ID, h.Mode, h.Epoch, h.LocalKeyID, h.Salt, h.Memory, h.Iterations, h.Parallelism}
	if h.Version >= 2 {
		fields = append(fields, h.KeyStorage)
	}
	data, _ := json.Marshal(fields)
	return data
}
func itemAAD(h store.VaultHeader, item store.VaultItem) []byte {
	data, _ := json.Marshal([]any{"burrow.vault.item", h.Version, h.ID, item.ID, item.Kind, item.Revision, item.Epoch})
	return data
}
func randomBytes(size int) []byte { data := make([]byte, size); rand.Read(data); return data }
func randomID() string            { return hex.EncodeToString(randomBytes(16)) }
func validID(id string) bool {
	data, err := hex.DecodeString(id)
	return err == nil && len(data) == 16 && id == strings.ToLower(id)
}
func (m *Manager) localPath(id string) string {
	return filepath.Join(m.directory, "vault-keys", id+".key")
}

type localKey struct {
	Version     int
	VaultID, ID string
	Key         []byte
}

func (m *Manager) writeLocalKey(h store.VaultHeader, kek []byte) error {
	data, err := json.Marshal(localKey{Version: 1, VaultID: h.ID, ID: h.LocalKeyID, Key: kek})
	if err != nil {
		return err
	}
	defer clear(data)
	if h.KeyStorage == "keychain" {
		if m.keyStore == nil {
			return errors.New("此凭据库需要 macOS 钥匙串解锁，请使用配套备份恢复")
		}
		account := keychainAccount(h)
		if err := m.keyStore.Set(account, data); err != nil {
			return fmt.Errorf("保存钥匙串解锁密钥：%w", err)
		}
		verified, err := m.keyStore.Get(account)
		defer clear(verified)
		if err != nil {
			return fmt.Errorf("核对钥匙串解锁密钥：%w", err)
		}
		if !bytes.Equal(data, verified) {
			return errors.New("钥匙串解锁密钥保存后不匹配")
		}
		return nil
	}
	directory := filepath.Join(m.directory, "vault-keys")
	if err := localfs.MkdirAll(directory, 0700); err != nil {
		return err
	}
	f, err := localfs.OpenFile(m.localPath(h.LocalKeyID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	return errors.Join(syncDirectory(directory), syncDirectory(m.directory))
}
func keychainAccount(h store.VaultHeader) string {
	return "vault/" + h.ID + "/unlock/" + h.LocalKeyID
}

func (m *Manager) readLocalKey(h store.VaultHeader) ([]byte, error) {
	var data []byte
	var err error
	if h.KeyStorage == "keychain" {
		if m.keyStore == nil {
			return nil, errors.New("此凭据库需要 macOS 钥匙串解锁，请使用配套备份恢复")
		}
		data, err = m.keyStore.Get(keychainAccount(h))
	} else {
		path := m.localPath(h.LocalKeyID)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return nil, fmt.Errorf("读取本机解锁密钥：%w", statErr)
		}
		if !info.Mode().IsRegular() || info.Size() > 4096 {
			return nil, errors.New("invalid local unlock key file")
		}
		if err := localfs.CheckPrivate(path, info); err != nil {
			return nil, err
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("读取自动解锁密钥：%w", err)
	}
	defer clear(data)
	var saved localKey
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	if saved.Version != 1 || saved.ID != h.LocalKeyID || saved.VaultID != h.ID || len(saved.Key) != 32 {
		clear(saved.Key)
		return nil, errors.New("本机解锁密钥与凭据库不匹配")
	}
	return saved.Key, nil
}

// Backup pairs the consistent database snapshot with its active local wrapping
// key. Holding the read lock prevents a protection change between the two.
func (m *Manager) Backup(ctx context.Context, destination string) (result error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.key) != 32 {
		return ErrLocked
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	if err := localfs.MkdirAll(destination, 0700); err != nil {
		return err
	}
	if err := m.db.Backup(ctx, filepath.Join(destination, "burrow.db")); err != nil {
		return err
	}
	if m.snapshot.Header.Mode == "local" {
		// Use the in-memory DEK to give this consistent snapshot an independent,
		// portable wrapping key. This also works if the original store is unavailable.
		db, err := store.Open(ctx, filepath.Join(destination, "burrow.db"))
		if err != nil {
			return err
		}
		defer func() {
			result = errors.Join(result, db.Close(), syncDirectory(destination))
		}()
		saved, err := db.ReadVault(ctx)
		if err != nil {
			return err
		}
		portable := saved
		if portable.Header.Version >= 2 {
			portable.Header.KeyStorage = "file"
		}
		portable.Header.LocalKeyID = randomID()
		portable.Header.Nonce = randomBytes(24)
		portableKey := randomBytes(32)
		defer clear(portableKey)
		target := &Manager{directory: destination}
		if err := target.writeLocalKey(portable.Header, portableKey); err != nil {
			return err
		}
		aead, err := chacha20poly1305.NewX(portableKey)
		if err != nil {
			return err
		}
		portable.Header.WrappedKey = aead.Seal(nil, portable.Header.Nonce, m.key, headerAAD(portable.Header))
		if err := db.ReplaceVault(ctx, saved, portable, nil); err != nil {
			return err
		}
		return nil
	}
	return syncDirectory(destination)
}
