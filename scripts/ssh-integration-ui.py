#!/usr/bin/env python3
"""Drive the real native UI against the disposable Docker SSH fixture.

Requires BURROW_TEST_DIR and BURROW_TEST_ADDRESS. It uses Go overlays for
automation and configuration isolation; release sources and user data stay intact.
"""

import argparse
import base64
import hashlib
import sqlite3
import uuid
from datetime import datetime, timezone
from contextlib import contextmanager
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time


PROBE_SOURCE = r'''package desktop

import (
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "sort"
    "strconv"

    "github.com/egoist/mygo"
    "github.com/egoist/mygo/ui"
)

var dockerIntegrationApp *app
var dockerIntegrationContext *ui.Context

type dockerProfileInfo struct {
    ID, Port int
    Name, User, Address, Auth, Protocol, UUID string
}

type dockerGroupInfo struct {
    ID int
    Name string
    Hosts []hostID
    Collapsed bool
}

func dockerIntegrationGroups(a *app, scope section) []dockerGroupInfo {
    groups := []dockerGroupInfo{}
    for _, g := range a.groups {
        if g.scope == scope {
            groups = append(groups, dockerGroupInfo{ID:g.id, Name:g.name, Hosts:g.hosts, Collapsed:g.collapsed})
        }
    }
    return groups
}

func dockerIntegrationProfiles(a *app, scope section) []dockerProfileInfo {
    profiles := []dockerProfileInfo{}
    for id, h := range a.hosts {
        files := h.protocol == "sftp" || h.protocol == "ftp"
        if files != (scope == sectionFiles) { continue }
        profiles = append(profiles, dockerProfileInfo{ID:int(id), Port:h.port,
            Name:h.name, User:h.user, Address:h.address, Auth:h.session.Auth, Protocol:h.protocol, UUID:h.session.ID})
    }
    return profiles
}

func dockerIntegrationConfigDir() (string, error) {
    path := os.Getenv("BURROW_INTEGRATION_CONFIG")
    if !filepath.IsAbs(path) { return "", errors.New("isolated integration configuration is required") }
    return path, nil
}

type dockerBounds struct {
    X float32 `json:"x"`
    Y float32 `json:"y"`
    Width float32 `json:"width"`
    Height float32 `json:"height"`
}

type dockerFileEntry struct {
    Name, Path, Permissions, Owner string
    Directory, Symlink bool
    Size int64
}

type dockerFilePane struct {
    Path, Error string
    Loading, Remote bool
    HistoryIndex int
    History, Selection []string
    Entries []dockerFileEntry
    Bounds dockerBounds
}

func dockerIntegrationPane(p *filePaneState) dockerFilePane {
    result := dockerFilePane{Path:p.path, Error:p.errorText.String(), Loading:p.loading, Remote:p.remote,
        HistoryIndex:p.historyIndex, History:append([]string{}, p.history...),
        Selection:[]string{}, Entries:[]dockerFileEntry{}}
    for selected := range p.table.Selection.All() { result.Selection = append(result.Selection, selected) }
    sort.Strings(result.Selection)
    for _, e := range p.directories[p.path] {
        result.Entries = append(result.Entries, dockerFileEntry{Name:e.name, Path:e.path, Directory:e.dir,
            Symlink:e.symlink, Size:e.size, Permissions:e.permissions, Owner:e.owner})
    }
    if c := dockerIntegrationContext; c != nil {
        bounds := p.focus.Bounds(c)
        result.Bounds = dockerBounds{bounds.X, bounds.Y, bounds.W, bounds.H}
    }
    return result
}

func dockerIntegrationSnapshot() {
    a := dockerIntegrationApp
    if a == nil { return }
    type sessionInfo struct {
        ID, Host, State, Connection, Generation, Columns, Rows, TerminalColumns, TerminalRows int
        QueueLength, OpeningSession int
        Focused, Closing, ShellReturned, RunCanceled, OwnerPresent, OwnerDone bool
        Name, Address, Text string
    }
    type fileSessionInfo struct {
        ID, Host, State, Connection, Generation int
        Selected, Temporary, Loading, LocalLoading bool
        Name, Protocol, Path, Error, RestoreID, LocalPath, LocalError string
        Entries, LocalEntries []dockerFileEntry
        RemoteSelection, LocalSelection []string
        RemoteBounds, LocalBounds dockerBounds
        Local, Remote dockerFilePane
    }
    type transferInfo struct {
        ID, ViewID, SessionID, State, BatchID, TemporaryCount int
        RestoreID, ViewRestoreID, Name, SourcePath, TargetPath, Error, ErrorText string
        SourceRemote, RunPresent, CleanupPending, RemoveRequested bool
        Transferred, Total, RunTransferred, RunTotal int64
        Speed float64
    }
    out := struct {
        Sessions []sessionInfo
        FileSessions []fileSessionInfo
        TransferTasks []transferInfo
        SSHProfiles, FileProfiles []dockerProfileInfo
        SSHGroups, FileGroups []dockerGroupInfo
        Prompts, Tabs, FileTabs, Section, ActiveSection, CatalogVersion, Transfers, ClosedTabs int
        DraggedFileSession, PaneDragKind, PaneDragPhase int
        LocalOpen, ActiveRemote, DragActive, FileDragActive, PaneDragActive, SettingsOpen, Busy bool
        FileDragPendingDirectory, FileDragPendingDisconnect, FileDragCancelRequested bool
        FileDragEntryPaths []string
        SettingsRecording, CatalogError, Locale, AppName string
    }{Prompts: len(a.sshPrompts), Tabs: len(a.tabs), FileTabs:len(a.fileSessions), Section:int(a.section),
      ActiveSection:int(a.activeSection()), Transfers:len(a.transfers.tasks), LocalOpen:a.files.localOpen,
      ActiveRemote:a.files.activeRemote, ClosedTabs:len(a.closedTabs), SettingsOpen:a.settingsUI.open,
      Locale:mygo.App.Locale(), AppName:mygo.App.Name(),
      SettingsRecording:a.settingsUI.recording, Sessions:[]sessionInfo{}, FileSessions:[]fileSessionInfo{},
      TransferTasks:[]transferInfo{}, FileDragEntryPaths:[]string{},
      SSHProfiles:dockerIntegrationProfiles(a, sectionSSH), FileProfiles:dockerIntegrationProfiles(a, sectionFiles),
      SSHGroups:dockerIntegrationGroups(a, sectionSSH), FileGroups:dockerIntegrationGroups(a, sectionFiles)}
    if err := a.catalogConfig.loadErr; err != nil { out.CatalogError = err.Error() }
    out.CatalogVersion = 1 // Store.Open has validated the schema version.
    out.Busy = a.interactionBlocked() || a.savingSession() || a.siteDeletion != nil
    for _, tab := range a.tabs {
        for _, s := range tab.root.sessions() {
            item := sessionInfo{ID:s.id, Host:int(s.host), State:int(s.state),
                Columns:s.columns, Rows:s.rows,
                Focused:tab == a.currentTab() && tab.focused == s, Closing:s.closing,
                Name:s.endpoint.name, Address:s.endpoint.address}
            item.OpeningSession = -1
            if s.connection != nil {
                item.Connection, item.Generation = s.connection.id, s.connection.generation
                item.QueueLength = len(s.connection.queue)
            }
            if s.ssh != nil {
                item.ShellReturned = s.ssh.conn != nil
                item.RunCanceled = s.ssh.ctx.Err() != nil
            }
            if s.connection != nil {
                if owner := s.connection.connection; owner != nil {
                    item.OwnerPresent = true
                    select { case <-owner.Done(): item.OwnerDone = true; default: }
                }
                if opening := s.connection.opening; opening != nil {
                    for _, otherTab := range a.tabs {
                        for _, other := range otherTab.root.sessions() {
                            if other.ssh != nil && other.ssh.request == opening { item.OpeningSession = other.id }
                        }
                    }
                }
            }
            if s.terminal != nil {
                item.Text = s.terminal.view.Text()
                item.TerminalColumns, item.TerminalRows = s.terminal.view.Size()
            } else {
                for _, line := range s.output { for _, span := range line { item.Text += span.Text }; item.Text += "\n" }
            }
            out.Sessions = append(out.Sessions, item)
        }
    }
    for i, s := range a.fileSessions {
        local, remote := dockerIntegrationPane(s.local), dockerIntegrationPane(s.remote)
        item := fileSessionInfo{ID:s.id, Host:int(s.host), State:int(s.state), Selected:i==a.activeFile,
            Temporary:s.temporary, Loading:s.remote.loading, Name:s.endpoint.name, Protocol:s.endpoint.protocol,
            Path:s.remote.path, Error:s.remote.errorText.String(), RestoreID:s.restoreID,
            Local:local, Remote:remote, LocalPath:local.Path, LocalLoading:local.Loading, LocalError:local.Error,
            LocalEntries:local.Entries, Entries:remote.Entries,
            RemoteSelection:remote.Selection, LocalSelection:local.Selection,
            RemoteBounds:remote.Bounds, LocalBounds:local.Bounds}
        if !s.errorText.Empty() { item.Error = s.errorText.String() }
        if s.connection != nil { item.Connection, item.Generation = s.connection.id, s.connection.generation }
        out.FileSessions = append(out.FileSessions, item)
    }
    for _, task := range a.transfers.tasks {
        item := transferInfo{ID:task.id, SessionID:task.session.id, RestoreID:task.session.restoreID,
            Name:task.entry.name, State:int(task.state), SourcePath:task.sourcePath,
            TargetPath:task.targetPath, Error:task.errorText.String(), ErrorText:task.errorText.String(), SourceRemote:task.sourceRemote,
            RunPresent:task.run != nil, CleanupPending:task.cleanupPending, RemoveRequested:task.removeRequested,
            Transferred:task.transferred, Total:task.total, Speed:task.speed, TemporaryCount:-1}
        if task.view != nil { item.ViewID, item.ViewRestoreID = task.view.id, task.view.restoreID }
        if task.job != nil {
            item.BatchID = task.job.batch.id
            // A running worker exclusively owns the manifest and staging map.
            if task.run == nil { item.TemporaryCount = len(task.job.temporary) }
        }
        if task.run != nil { item.RunTransferred, item.RunTotal = task.run.bytes.Load(), task.run.total.Load() }
        out.TransferTasks = append(out.TransferTasks, item)
    }
    if drag := a.files.drag; drag != nil {
        out.DragActive, out.FileDragActive, out.DraggedFileSession = true, true, drag.session.id
        out.FileDragPendingDirectory = drag.publishDirectory != nil
        out.FileDragPendingDisconnect = drag.publishDisconnect != nil
        out.FileDragCancelRequested = drag.cancelRequested()
        for _, entry := range drag.entries { out.FileDragEntryPaths = append(out.FileDragEntryPaths, entry.path) }
    }
    if drag := a.drag; drag != nil {
        out.DragActive, out.PaneDragActive = true, true
        out.PaneDragKind, out.PaneDragPhase = int(drag.kind), int(drag.phase)
    }
    data, err := json.Marshal(out)
    if err != nil { panic(err) }
    if err := os.WriteFile(filepath.Join(os.Getenv("BURROW_TEST_DIR"), "native-observation.json"), data, 0600); err != nil { panic(err) }
}

// These existing model actions exercise controller integration only. Snapshot
// is read-only; native end-to-end cases drive the actual UI instead.
func dockerIntegrationMenu(items []*mygo.MenuItem) []*mygo.MenuItem {
    return append(items, &mygo.MenuItem{Label:"Integration", Submenu:[]*mygo.MenuItem{
            {Label:"Snapshot", Click:func(*mygo.MenuItem,*mygo.Window){ dockerIntegrationSnapshot() }},
            {Label:"Focus window", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil { a.window.Show(); mygo.App.Focus(); a.window.Focus() }
            }},
            {Label:"Disconnect", Click:func(*mygo.MenuItem,*mygo.Window){
                a:=dockerIntegrationApp; if a!=nil && a.currentTab()!=nil { disconnectSession(a.currentTab().focused); a.window.Invalidate() }
            }},
            {Label:"Reconnect", Click:func(*mygo.MenuItem,*mygo.Window){
                a:=dockerIntegrationApp; if a!=nil && a.currentTab()!=nil { a.connectSession(a.currentTab().focused); a.window.Invalidate() }
            }},
            {Label:"Large window", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil { a.window.SetContentSize(1560,900) }
            }},
            {Label:"Split source twice", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil && a.currentTab()!=nil {
                    source:=a.currentTab().focused
                    a.splitSession(source,false)
                    a.splitSession(source,true)
                    a.window.Invalidate()
                }
            }},
            {Label:"Create catalog record", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil { a.startHostDraft(); a.window.Invalidate() }
            }},
            {Label:"Edit catalog fixture", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil {
                    for _, group := range a.catalogGroups() { if len(group.hosts)>0 { a.startEditHostDraft(group.hosts[0]); break } }
                    a.window.Invalidate()
                }
            }},
            {Label:"Delete catalog fixture", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil {
                    for _, group := range a.catalogGroups() { if len(group.hosts)>0 {
                        h := a.hosts[group.hosts[0]]
                        a.page().draft = hostDraft{id:group.hosts[0], name:h.name, deleting:true, deleteProfileID:h.session.ID, files:a.section==sectionFiles}
                        a.page().addHostOpen = true
                        break
                    } }
                    a.window.Invalidate()
                }
            }},
            {Label:"Quit", Click:func(*mygo.MenuItem,*mygo.Window){ mygo.App.Quit() }},
    }})
}

func init() {
    root, err := dockerIntegrationConfigDir()
    if err != nil { panic(err) }
    mygo.App.SetPath(mygo.PathUserData, filepath.Join(root, "Burrow"))
    mygo.App.SetPath(mygo.PathCache, filepath.Join(root, "cache"))
    mygo.App.SetPath(mygo.PathLogs, filepath.Join(root, "logs"))
    if err := os.Setenv("MYGO_LOCALE", "zh-Hans"); err != nil { panic(err) }
    if err := os.WriteFile(filepath.Join(root, "app.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil { panic(err) }
}
'''


def ensure_unlocked():
    result = subprocess.run(["/usr/sbin/ioreg", "-n", "Root", "-d", "1", "-a"], capture_output=True, check=True)
    if plistlib.loads(result.stdout)["IOConsoleLocked"]:
        raise RuntimeError("Mac is locked; unlock the Mac before running native UI integration")


def prepare(repo, task_dir, address):
    host, port = address.rsplit(":", 1)
    if host not in ("127.0.0.1", "localhost", "[::1]"):
        raise RuntimeError("the UI fixture must use a loopback address")
    overlay_dir = task_dir / "native-overlay"
    overlay_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    config_root = Path(tempfile.mkdtemp(prefix="native-config-", dir=task_dir))
    config_dir = config_root / "Burrow"
    config_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    local_dir = task_dir / "ui-files"
    local_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    for name in ("cache", "logs"):
        (config_root / name).mkdir(mode=0o700)
    # Seed pre-vault credentials in a disposable directory; migrate through the isolated provider.
    now = datetime.now(timezone.utc).isoformat()
    key_ids = {}
    with sqlite3.connect(config_dir / "burrow.db") as db:
        db.executescript((repo / "internal/store/schema.sql").read_text())
        (config_dir / "keys").mkdir(mode=0o700)
        for filename in ("key_plain", "key_encrypted"):
            key_id = str(uuid.uuid4())
            key_ids[filename] = key_id
            source = task_dir / filename
            target = config_dir / "keys" / key_id
            shutil.copyfile(source, target)
            os.chmod(target, 0o600)
            public = (task_dir / (filename + ".pub")).read_bytes()
            (config_dir / "keys" / (key_id + ".pub")).write_bytes(public)
            blob = base64.b64decode(public.split()[1])
            fingerprint = "SHA256:" + base64.b64encode(hashlib.sha256(blob).digest()).decode().rstrip("=")
            db.execute("INSERT INTO managed_keys VALUES(?,?,?,?,?,?,?,?,?)",
                       (key_id, filename, "Ed25519", fingerprint, str(target), "", 256, now, public))
        for group_order, scope in enumerate(("ssh", "files")):
            group_id = str(uuid.uuid4())
            db.execute("INSERT INTO catalog_groups VALUES(?,?,?,?,?)",
                       (group_id, scope, "Docker integration", group_order, 0))
            for order, (name, auth, key) in enumerate([
                ("Docker Password", "password", ""),
                ("Docker Plain Key", "key", "key_plain"),
                ("Docker Encrypted Key", "key", "key_encrypted"),
                ("Docker Agent", "agent", ""),
            ]):
                profile_id = str(uuid.uuid4())
                key_id = key_ids.get(key)
                value = dict(id=profile_id, auth=auth, protocol="ssh" if scope == "ssh" else "sftp",
                             name=name, user="burrow", host=host.strip("[]"), port=int(port),
                             os=0, system="Linux", createdAt=now, updatedAt=now)
                if key_id:
                    value["keyId"] = key_id
                if scope == "ssh":
                    value["ssh"] = dict(keepAlive=30)
                else:
                    value["files"] = dict(encoding="UTF-8")
                db.execute("INSERT INTO profiles VALUES(?,?,?,?,?,?)",
                           (profile_id, group_id, order, key_id, None, json.dumps(value)))
        db.execute("INSERT INTO settings VALUES(?,?)",
                   ("preferences", json.dumps(dict(version=1, defaultLocalDir=str(local_dir)))))
    os.chmod(config_dir / "burrow.db", 0o600)
    mapping = {}
    def replace_once(content, before, after):
        if content.count(before) != 1:
            raise RuntimeError("integration overlay source no longer matches; isolation was not applied")
        return content.replace(before, after, 1)

    original = repo / "internal/desktop/app.go"
    replacement = overlay_dir / "app.go"
    replacement.write_text(replace_once(original.read_text(),
        "func (a *app) view(c *ui.Context) {",
        "func (a *app) view(c *ui.Context) {\n dockerIntegrationApp = a\n dockerIntegrationContext = c"))
    mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/data_paths.go"
    replacement = overlay_dir / "data_paths.go"
    replacement.write_text('''package desktop
import (
 "path/filepath"
 "github.com/egoist/mygo"
)
func configureDataDirectory() (string,error) {
 root,err:=dockerIntegrationConfigDir();if err!=nil{return "",err}
 directory:=filepath.Join(root,"Burrow")
 mygo.App.SetName("Burrow Integration")
 mygo.App.SetPath(mygo.PathUserData,directory)
 return directory,nil
}
func legacyCredentialStore() mygo.SecretsModule {return mygo.Secrets.Service("dev.burrow.app.integration")}
''')
    mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/application_security.go"
    replacement = overlay_dir / "application_security.go"
    content = original.read_text()
    content = replace_once(content, "import (", 'import (\n "os"\n "path/filepath"')
    content = replace_once(content, 'a.security.status = "migration"\n\t\treturn', '''a.security.status = "migration"
        catalog,err:=a.store.ReadCatalog(a.ctx)
        if err!=nil {a.security.errorText=locales.ErrorText(err);return}
        phrase,err:=os.ReadFile(filepath.Join(os.Getenv("BURROW_TEST_DIR"),"key_passphrase"))
        if err!=nil {a.security.errorText=locales.ErrorText(err);return}
        a.security.migrationPhrases=map[string]string{}
        for _,key:=range catalog.Keys {
            if key.Name=="key_encrypted" {a.security.migrationPhrases[key.ID]=strings.TrimSpace(string(phrase))}
        }
        clear(phrase)
        a.submitStartup()
        return''')
    content = replace_once(content, '"path/filepath"', '"path/filepath"\n "strings"')
    replacement.write_text(content)
    mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/ssh_terminal.go"
    replacement = overlay_dir / "ssh_terminal.go"
    replacement.write_text(replace_once(original.read_text(),
        'Label(session.endpoint.name + locales.MessageSshTerminal())',
        'Label(fmt.Sprintf("%s%s #%d", session.endpoint.name, locales.MessageSshTerminal(), session.id))'))
    mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/app_actions.go"
    content = replace_once(original.read_text(), "menu := mygo.NewMenu([]*mygo.MenuItem{",
                           "menu := mygo.NewMenu(dockerIntegrationMenu([]*mygo.MenuItem{")
    content = replace_once(content, "\n\t})\n\tmygo.App.SetMenu(menu)", "\n\t}))\n\tmygo.App.SetMenu(menu)")
    replacement = overlay_dir / "app_actions.go"
    replacement.write_text(content)
    mapping[str(original)] = str(replacement)

    module = json.loads(subprocess.check_output(["go", "list", "-m", "-json", "github.com/egoist/mygo"],
                                               cwd=repo, text=True))
    if module.get("Version") != "v0.7.3":
        raise RuntimeError("native integration isolation must be reviewed for the pinned MyGo version")
    # Go disallows overlays beneath GOMODCACHE. Copy the pinned SDK unchanged
    # into this run, then overlay its two isolation seams through a local modfile.
    mygo_dir = Path(tempfile.mkdtemp(prefix="mygo-sdk-", dir=overlay_dir))
    shutil.copytree(Path(module["Dir"]), mygo_dir, dirs_exist_ok=True)
    modfile = (repo / "go.mod").read_text()
    for module_name, relative in (("github.com/egoist/mygo/burrow-terminal", "third_party/mygo-terminal"),
                                  ("github.com/pkg/sftp", "third_party/pkg-sftp")):
        modfile = replace_once(modfile, "replace " + module_name + " => ./" + relative,
                               "replace " + module_name + " => " + json.dumps(str(repo / relative)))
    modfile += "\nreplace github.com/egoist/mygo => " + json.dumps(str(mygo_dir)) + "\n"
    (overlay_dir / "integration.mod").write_text(modfile)
    shutil.copyfile(repo / "go.sum", overlay_dir / "integration.sum")
    # The fixed automation plugin formats keys on its socket goroutine. Pointer
    # keys must expose identity, not race with layout or reveal pane contents.
    original = mygo_dir / "plugins/automation/protocol.go"
    content = replace_once(original.read_text(), '\n\t"strconv"', '\n\t"reflect"\n\t"strconv"')
    content = replace_once(content, '\treturn fmt.Sprint(k)', '''\tvalue := reflect.ValueOf(k)
    if value.Kind() == reflect.Pointer {
        return fmt.Sprintf("%T@%x", k, value.Pointer())
    }
    return fmt.Sprint(k)''')
    replacement = overlay_dir / "automation-protocol.go"
    replacement.write_text(content)
    mapping[str(original)] = str(replacement)
    original = mygo_dir / "paths.go"
    content = original.read_text()
    if content.count("os.UserConfigDir()") != 4:
        raise RuntimeError("MyGo configuration paths changed; isolation was not applied")
    content = content.replace("os.UserConfigDir()", "dockerIntegrationUserConfigDir()")
    content += '''
func dockerIntegrationUserConfigDir() (string, error) {
    root := os.Getenv("BURROW_INTEGRATION_CONFIG")
    if !filepath.IsAbs(root) { return "", fmt.Errorf("isolated integration configuration is required") }
    return root, nil
}
'''
    replacement = overlay_dir / "mygo-paths.go"
    replacement.write_text(content)
    mapping[str(original)] = str(replacement)

    # Native Keychain integration is deliberately excluded. Keep the public
    # MyGo Secrets API and validation, replacing only its Darwin provider with
    # a file store confined to this run so restart scenarios retain credentials.
    original = mygo_dir / "internal/darwin/secrets.go"
    replacement = overlay_dir / "mygo-secrets-darwin.go"
    replacement.write_text(r'''//go:build darwin

package darwin

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sync"

    "github.com/egoist/mygo/internal/platform"
)

type secrets struct{}
var integrationSecretsMu sync.Mutex

func integrationSecrets() (string, map[string][]byte, error) {
    root := os.Getenv("BURROW_INTEGRATION_CONFIG")
    if !filepath.IsAbs(root) { return "", nil, fmt.Errorf("isolated integration credential directory is required") }
    name := filepath.Join(root, "integration-secrets.json")
    values := map[string][]byte{}
    data, err := os.ReadFile(name)
    if errors.Is(err, os.ErrNotExist) { return name, values, nil }
    if err != nil { return "", nil, err }
    if err := json.Unmarshal(data, &values); err != nil { return "", nil, err }
    if values == nil { return "", nil, errors.New("integration credential store is null") }
    return name, values, nil
}

func writeIntegrationSecrets(name string, values map[string][]byte) (result error) {
    data, err := json.Marshal(values)
    if err != nil { return err }
    defer clear(data)
    file, err := os.CreateTemp(filepath.Dir(name), ".integration-secrets-*")
    if err != nil { return err }
    defer func() {
        if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) { result = errors.Join(result, err) }
    }()
    _, writeErr := file.Write(data)
    if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil { return err }
    return os.Rename(file.Name(), name)
}

func (secrets) SetSecret(service, account string, secret []byte) error {
    integrationSecretsMu.Lock()
    defer integrationSecretsMu.Unlock()
    name, values, err := integrationSecrets()
    if err != nil { return err }
    values[service+"\x00"+account] = append([]byte{}, secret...)
    return writeIntegrationSecrets(name, values)
}

func (secrets) Secret(service, account string) ([]byte, error) {
    integrationSecretsMu.Lock()
    defer integrationSecretsMu.Unlock()
    _, values, err := integrationSecrets()
    if err != nil { return nil, err }
    value, exists := values[service+"\x00"+account]
    if !exists { return nil, platform.ErrSecretNotFound }
    return append([]byte{}, value...), nil
}

func (secrets) DeleteSecret(service, account string) error {
    integrationSecretsMu.Lock()
    defer integrationSecretsMu.Unlock()
    name, values, err := integrationSecrets()
    if err != nil { return err }
    key := service+"\x00"+account
    if _, exists := values[key]; !exists { return nil }
    delete(values, key)
    return writeIntegrationSecrets(name, values)
}
''')
    mapping[str(original)] = str(replacement)
    original = repo / "main.go"
    replacement = overlay_dir / "main.go"
    content = replace_once(original.read_text(), "import (", 'import (\n "os"\n "path/filepath"\n _ "github.com/egoist/mygo/plugins/automation"')
    content = replace_once(content, "if err := desktop.Run(); err != nil {", '''err := desktop.Run()
    status := "ok"
    if err != nil { status = "error" }
    if writeErr := os.WriteFile(filepath.Join(os.Getenv("BURROW_INTEGRATION_CONFIG"), "app-exit-status"), []byte(status), 0600); writeErr != nil { log.Fatal(writeErr) }
    if err != nil {''')
    replacement.write_text(content)
    mapping[str(original)] = str(replacement)
    extra = overlay_dir / "integration_probe.go"
    extra.write_text(PROBE_SOURCE)
    mapping[str(repo / "internal/desktop/zz_docker_integration_probe.go")] = str(extra)
    overlay = overlay_dir / "overlay.json"
    overlay.write_text(json.dumps(dict(Replace=mapping)))
    return overlay, config_root


@contextmanager
def isolated_agent(task_dir):
    agent_socket = task_dir / ("native-agent-" + str(os.getpid()) + ".sock")
    process = subprocess.Popen(["ssh-agent", "-D", "-a", str(agent_socket)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        deadline = time.monotonic() + 10
        while not agent_socket.exists() and time.monotonic() < deadline:
            if process.poll() is not None:
                raise RuntimeError("isolated SSH agent failed to start")
            time.sleep(0.05)
        if not agent_socket.exists():
            raise RuntimeError("isolated SSH agent socket did not appear")
        env = dict(os.environ, SSH_AUTH_SOCK=str(agent_socket))
        result = subprocess.run(["ssh-add", str(task_dir / "key_plain")], env=env, stdin=subprocess.DEVNULL,
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode:
            raise RuntimeError("fixture key could not be added to isolated SSH agent")
        yield str(agent_socket)
    finally:
        if process.poll() is None:
            process.terminate()
            process.wait(timeout=5)


class Driver:
    def __init__(self, connection, task_dir, config_root):
        self.connection = connection
        self.reader = connection.makefile("rb")
        self.task_dir = task_dir
        self.config_root = config_root
        self.counter = 0
        fixture = json.loads((task_dir / "server.json").read_text())
        if fixture["address"] != os.environ["BURROW_TEST_ADDRESS"] or not fixture["container"].startswith("burrow-integration-"):
            raise RuntimeError("native fixture metadata does not match the requested local server")
        self.container = fixture["container"]

    def call(self, method, params=None):
        if method in ("input", "act"):
            self.window_focused()
        self.counter += 1
        request = dict(id=self.counter, method=method)
        if params is not None:
            request["params"] = params
        self.connection.sendall(json.dumps(request).encode() + b"\n")
        try:
            response = json.loads(self.reader.readline())
        except TimeoutError:
            raise RuntimeError("automation timed out: " + method) from None
        if "error" in response:
            # Never include request params, which may contain a credential.
            raise RuntimeError(method + ": " + response["error"]["code"])
        return response.get("result", {})

    def nodes(self):
        return self.call("tree").get("nodes", [])

    def node(self, label, role=None):
        for node in self.nodes():
            matches = node.get("label") == label
            if role == "listItem":
                matches = matches or node.get("label", "").startswith(label + "，")
            if matches and (role is None or node.get("role") == role):
                return node
        return None

    def wait(self, description, check, timeout=15):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            result = check()
            if result:
                return result
            time.sleep(0.05)
        raise RuntimeError("timeout: " + description)

    def click(self, label, role="button"):
        try:
            node = self.wait("element " + label, lambda: self.node(label, role))
        except RuntimeError:
            safe = [(n.get("role"), n.get("label"), n.get("actions")) for n in self.nodes()
                    if "关闭" in n.get("label", "") or n.get("label") in ("Docker Password", "输入 SSH 密码", "信任并连接", "取消", "连接")]
            print("UI selector candidates (labels only): " + repr(safe), file=sys.stderr)
            raise
        self.call("act", dict(id=node["id"], action="press"))

    def input(self, *events):
        self.call("input", dict(events=list(events)))

    def key(self, key):
        self.input(dict(type="key", key=key))

    def window_focused(self):
        if not any(w.get("focused") for w in self.call("windows").get("windows", [])):
            self.menu("Focus window")
            self.wait("native window focus", lambda: any(w.get("focused") for w in self.call("windows").get("windows", [])))


    def text(self, text):
        self.input(dict(type="text", text=text))

    def menu(self, name):
        self.call("menu", dict(path=["Integration", name]))

    def quit(self):
        state = self.snapshot()
        name = state["AppName"]
        label = "退出" + name if state["Locale"] == "zh-Hans" else "Quit " + name
        self.call("menu", dict(path=["Burrow", label]))

    def snapshot(self):
        self.menu("Snapshot")
        return json.loads((self.task_dir / "native-observation.json").read_text())

    def sessions(self):
        return self.snapshot().get("Sessions") or []

    def focused(self):
        return next((s for s in self.sessions() if s["Focused"]), None)

    def state(self, state, count=1):
        def check():
            if self.snapshot()["Section"] != 0:
                return None
            sessions = self.sessions()
            return sessions if len(sessions) == count and all(s["State"] == state for s in sessions) else None
        return self.wait("session state", check)

    def shared_shells(self, count):
        def check():
            state = self.snapshot()
            if state["Prompts"]:
                raise RuntimeError("a shared SSH connection requested credentials again")
            sessions = state.get("Sessions") or []
            if any(s["State"] == 3 for s in sessions):
                raise RuntimeError("a shared SSH shell failed to open")
            if state["Section"] != 0:
                return None
            return sessions if len(sessions) == count and all(s["State"] == 1 for s in sessions) else None
        return self.wait("shared SSH shells without authentication", check)

    def established_connections(self, container, port):
        command = "awk '$2 ~ /:" + format(port, "04X") + "$/ && $4 == \"01\" { n++ } END { print n+0 }' /proc/net/tcp /proc/net/tcp6"
        return int(subprocess.check_output(["docker", "exec", container, "sh", "-c", command], text=True).strip())

    def server_connections(self):
        # These observations are confined to the named disposable local fixture.
        logs = subprocess.check_output(["docker", "logs", self.container], stderr=subprocess.STDOUT, text=True)
        accepted = sum("Accepted " in line and " for burrow " in line for line in logs.splitlines())
        established = self.established_connections(self.container, 22)
        return accepted, established

    def fill(self, label, value):
        node = self.wait("field " + label, lambda: self.node(label, "textField"))
        self.call("act", dict(id=node["id"], action="setValue", text=value))

    def settled(self):
        # Catalog writes finish asynchronously; the application drops shortcuts
        # and rejects further catalog changes until they do.
        self.wait("application accepts input", lambda: not self.snapshot()["Busy"])

    def page(self, label, section):
        self.settled()
        self.key(("Cmd+Alt+1", "Cmd+Alt+2")[section])
        self.wait("selected page " + label, lambda: self.snapshot()["Section"] == section)

    def profiles(self, section):
        return self.snapshot()["SSHProfiles" if section == 0 else "FileProfiles"]

    def file(self):
        return next((s for s in self.snapshot().get("FileSessions") or [] if s["Selected"]), None)

    def files_ready(self, expected=None, no_auth=False):
        def check():
            self.window_focused()
            state = self.snapshot()
            if no_auth and state["Prompts"]:
                raise RuntimeError("SSH/SFTP shortcut requested authentication again")
            selected = next((s for s in state.get("FileSessions") or [] if s["Selected"]), None)
            if selected and selected["State"] == 3:
                raise RuntimeError("file connection failed: " + selected["Error"])
            if not selected or selected["State"] != 1 or selected["Loading"] or selected["Error"]:
                return None
            if state["Section"] != 1 or self.node("远程文件") is None:
                return None
            tab = self.node(selected["Name"], "tab")
            if tab is None or "selected" not in tab.get("states", []):
                return None
            names = {e["Name"] for e in selected["Entries"] or []}
            return selected if expected is None or set(expected).issubset(names) else None
        try:
            return self.wait("remote directory result", check)
        except RuntimeError:
            state = self.snapshot()
            selected = self.file()
            labels = {"远程文件", selected["Name"] if selected else ""}
            print("Remote ready state: " + repr(dict(section=state["Section"], file=selected,
                  nodes=[(n.get("role"), n.get("label"), n.get("states"), n.get("bounds"))
                         for n in self.nodes() if n.get("label") in labels],
                  windows=[(w.get("focused"), w.get("visible")) for w in self.call("windows").get("windows", [])])), file=sys.stderr)
            raise

    def open_directory(self, name):
        try:
            item = self.wait("remote directory " + name, lambda: self.node(name, "listItem") or self.node(name, "row"))
        except RuntimeError:
            state = self.snapshot()
            file = self.file()
            print("Remote directory UI state: " + repr(dict(section=state["Section"], selected=file["Name"] if file else None,
                  path=file["Path"] if file else None, loading=file["Loading"] if file else None,
                  names=[e["Name"] for e in (file["Entries"] or [])] if file else [],
                  windows=[(w.get("focused"), w.get("visible")) for w in self.call("windows").get("windows", [])])), file=sys.stderr)
            labels = {"hello.txt", "你好.txt", "nested", "inside.txt", "denied", "远程文件", "本地文件"}
            print("Remote directory UI nodes: " + repr([(n.get("role"), n.get("label"), n.get("bounds"))
                  for n in self.nodes() if n.get("label") in labels]), file=sys.stderr)
            raise
        self.call("act", dict(id=item["id"], action="press"))
        self.call("idle", dict(timeout=5000))
        self.key("Enter")

    def close_file_tab(self, name):
        self.window_focused()
        self.call("idle", dict(timeout=5000))
        self.wait("file workspace before close", lambda: self.snapshot()["Section"] == 1)
        tab = self.wait("file tab", lambda: self.node(name, "tab"))
        bounds = tab["bounds"]
        before = self.snapshot()["FileTabs"]
        self.input(dict(type="click", x=bounds["x"] + bounds["width"] - 16,
                        y=bounds["y"] + bounds["height"] / 2))
        self.wait("file close confirmation or removal", lambda: self.snapshot()["FileTabs"] == before - 1
                  or self.node("关闭选项卡", "button"))
        if self.node("关闭选项卡", "button"):
            self.click("关闭选项卡")
        try:
            self.wait("closed file tab", lambda: self.snapshot()["FileTabs"] == before - 1)
        except RuntimeError:
            state = self.snapshot()
            print("File tab close state: " + repr(dict(before=before, after=state["FileTabs"], section=state["Section"],
                  ssh_tabs=state["Tabs"], bounds=bounds)), file=sys.stderr)
            raise

    def select_protocol(self, protocol):
        trigger = self.wait("protocol selector " + protocol, lambda: next((n for n in self.nodes()
                            if n.get("label") == protocol and "press" in n.get("actions", [])), None))
        self.call("act", dict(id=trigger["id"], action="press"))

    def create_file_site(self, name, protocol, address):
        self.page("SFTP / FTP", 1)
        self.menu("Create catalog record")
        self.select_protocol(protocol)
        host, port = address.rsplit(":", 1)
        for label, value in (("名称", name), ("主机", host.strip("[]")), ("用户名", "burrow"), ("端口", port)):
            self.fill(label, value)
        self.click("仅保存")
        self.wait("saved file site", lambda: any(p["Name"] == name and p["Protocol"] == protocol.lower() for p in self.profiles(1)))

    def writable_file_controls(self):
        for label in ("上传文件", "新建文件夹"):
            node = self.wait("file operation control " + label, lambda: self.node(label))
            if "disabled" in node.get("states", []):
                raise RuntimeError("a writable fixture directory disabled " + label)
        before = self.snapshot()
        self.key("Cmd+Shift+L")
        self.wait("local panel shortcut", lambda: self.snapshot()["LocalOpen"] != before["LocalOpen"])
        self.key("Cmd+Shift+L")
        self.wait("local panel restored", lambda: self.snapshot()["LocalOpen"] == before["LocalOpen"])
        if self.snapshot()["Transfers"] != before["Transfers"]:
            raise RuntimeError("toggling the local panel unexpectedly created a transfer task")

    def browse_directories(self):
        initial = self.files_ready({"hello.txt", "你好.txt", "nested", "denied"})
        self.open_directory("nested")
        nested = self.files_ready({"inside.txt"})
        if nested["Path"] == initial["Path"]:
            raise RuntimeError("entering a directory did not update the remote path")
        self.click("后退")
        self.files_ready({"hello.txt", "nested"})
        self.click("刷新")
        self.files_ready({"hello.txt", "你好.txt"})
        self.open_directory("denied")
        def failed_listing():
            state = self.file()
            return state if state and not state["Loading"] and state["Error"] else None
        failed = self.wait("visible directory permission error", failed_listing)
        if failed["State"] != 1 or failed["Path"] != initial["Path"]:
            raise RuntimeError("a directory error replaced the last successful path or disconnected a healthy file session")
        if not any(n.get("label", "").startswith("读取目录失败") for n in self.nodes()):
            raise RuntimeError("directory permission error is missing from the actual UI")
        self.click("刷新")
        self.files_ready({"hello.txt", "nested"})
        self.writable_file_controls()
        return initial["Path"]

    def run_file_protocols(self):
        from ssh_e2e_scenarios import run_file_browsing
        run_file_browsing(self)

    def run_concurrent_split(self):
        password = (self.task_dir / "password").read_text().strip()
        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.click("信任并连接")
        self.state(1)
        self.shell_ready()
        before = self.server_connections()
        self.window_focused()
        self.menu("Split source twice")
        def ready():
            self.window_focused()
            state = self.snapshot()
            if state["Prompts"]:
                raise RuntimeError("concurrent shared shells requested authentication again")
            shells = state.get("Sessions") or []
            if any(s["State"] == 3 for s in shells):
                raise RuntimeError("a concurrent shared shell failed")
            return shells if len(shells) == 3 and all(s["State"] == 1 for s in shells) else None
        try:
            shells = self.wait("three concurrent shared shells", ready, timeout=20)
        except RuntimeError:
            state = self.snapshot()
            fields = ("ID", "State", "Connection", "Generation", "QueueLength", "OpeningSession",
                      "ShellReturned", "RunCanceled", "OwnerPresent", "OwnerDone")
            top = subprocess.check_output(["docker", "top", self.container, "-eo", "comm"], text=True)
            print("Concurrent creation diagnostics: " + repr(dict(
                sessions=[{key:s[key] for key in fields} for s in state.get("Sessions") or []],
                bash_processes=sum(line.strip() == "bash" for line in top.splitlines()),
                windows=[(w.get("focused"), w.get("visible")) for w in self.call("windows").get("windows", [])])), flush=True)
            print("Paused for 30 seconds before isolated application cleanup", flush=True)
            time.sleep(30)
            raise
        if len({(s["Connection"], s["Generation"]) for s in shells}) != 1 or self.server_connections() != before:
            raise RuntimeError("concurrent split did not share one authenticated SSH transport")
        print("PASS native UI: three simultaneous shell requests queue on one authenticated transport")
        self.menu("Quit")

    def catalog_state(self, section):
        state = self.snapshot()
        profiles = state["SSHProfiles" if section == 0 else "FileProfiles"]
        identities = {profile["ID"]: profile["UUID"] for profile in state["SSHProfiles"] + state["FileProfiles"]}
        groups = state["SSHGroups" if section == 0 else "FileGroups"]
        # Array indices compact after deletion; persistent UUIDs define identity.
        return ([{key: value for key, value in profile.items() if key != "ID"} for profile in profiles],
                [dict(group, Hosts=[identities[index] for index in group["Hosts"] or []]) for group in groups])

    def add_catalog_record(self, name):
        self.settled()
        self.menu("Create catalog record")
        host, port = os.environ["BURROW_TEST_ADDRESS"].rsplit(":", 1)
        fields = (("名称", name), ("主机", host.strip("[]")), ("用户名", "burrow"), ("端口", port))

        def filled():
            # The opening form can replace a value set while it initializes;
            # an incomplete form is not saved and reports nothing.
            complete = True
            for label, value in fields:
                node = self.node(label, "textField")
                if node is None or node.get("value", "") != value:
                    self.fill(label, value)
                    complete = False
            return complete
        self.wait("catalog form values", filled)
        self.click("仅保存")

    def delete_catalog_fixture(self):
        self.settled()
        self.menu("Delete catalog fixture")
        def button():
            return next((n for n in self.nodes() if n.get("role") == "button" and n.get("label") == "删除"), None)
        target = self.wait("catalog delete confirmation", button)
        self.call("act", dict(id=target["id"], action="press"))

    def catalog_isolation(self, password):
        self.page("SSH", 0)
        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.state(1)
        self.shell_ready()
        before_files = self.profiles(1)
        before_file_catalog = self.catalog_state(1)
        before_ssh = self.profiles(0)
        original = next(p for p in before_ssh if p["Name"] == "Docker Password")
        self.add_catalog_record("Docker SSH Only")
        self.wait("saved SSH-only record", lambda: len(self.profiles(0)) == len(before_ssh) + 1)
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("adding an SSH host changed the file catalog")
        self.settled()
        self.menu("Edit catalog fixture")
        self.fill("名称", "Docker SSH Renamed")
        self.fill("主机", "127.0.0.2")
        self.click("保存")
        self.wait("saved SSH edit", lambda: next(p for p in self.profiles(0) if p["UUID"] == original["UUID"])["Name"] == "Docker SSH Renamed")
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("editing an SSH host changed the file catalog")
        if any(s["Name"] != original["Name"] or s["Address"] != original["Address"] for s in self.sessions()):
            raise RuntimeError("editing a catalog record changed an already connected session identity")
        self.delete_catalog_fixture()
        self.wait("deleted SSH record", lambda: all(p["UUID"] != original["UUID"] for p in self.profiles(0)))
        self.wait("deleted site's sessions closed", lambda: not self.sessions())
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("deleting an SSH host changed the file catalog")

        self.page("SFTP / FTP", 1)
        if self.snapshot()["FileTabs"]:
            raise RuntimeError("switching to files opened a file session without an explicit request")
        stable_ssh = self.catalog_state(0)
        self.add_catalog_record("Docker File Only")
        self.wait("saved file-only record", lambda: len(self.profiles(1)) == len(before_files) + 1)
        file_uuid = before_files[0]["UUID"]
        self.settled()
        self.menu("Edit catalog fixture")
        self.fill("名称", "Docker File Renamed")
        self.click("保存")
        self.wait("saved file edit", lambda: next(p for p in self.profiles(1) if p["UUID"] == file_uuid)["Name"] == "Docker File Renamed")
        self.delete_catalog_fixture()
        self.wait("deleted file record", lambda: all(p["UUID"] != file_uuid for p in self.profiles(1)))
        if self.catalog_state(0) != stable_ssh:
            raise RuntimeError("file catalog operations changed the SSH catalog")
        self.page("SSH", 0)
        print("PASS native UI: independent catalogs, stable running identity and deletion isolation")

    def open_host(self, name):
        self.call("idle", dict(timeout=5000))
        node = self.wait("host", lambda: self.node(name, "listItem"))
        self.call("act", dict(id=node["id"], action="focus"))
        self.call("idle", dict(timeout=5000))
        self.key("Enter")

    def secret(self, label, value):
        try:
            node = self.wait("credential field " + label, lambda: self.node(label, "textField"))
        except RuntimeError:
            state = self.snapshot()
            print("Credential state: " + repr(dict(tabs=state["Tabs"], prompts=state["Prompts"],
                  sessions=[(s["Host"], s["State"]) for s in state.get("Sessions") or []])), file=sys.stderr)
            print("Credential labels: " + repr([(n.get("role"), n.get("label")) for n in self.nodes()
                  if n.get("role") in ("textField", "dialog", "button")]), file=sys.stderr)
            raise
        self.call("act", dict(id=node["id"], action="setValue", text=value))
        self.click("连接")

    def focus_terminal(self, host="Docker Password", session_id=None):
        target = self.focused() if session_id is None else next(s for s in self.sessions() if s["ID"] == session_id)
        expected = host + " SSH 终端 #" + str(target["ID"]) if target else ""
        if not target:
            raise RuntimeError("terminal focus target missing")
        # The terminal appears a frame after the session reports connected.
        candidates = self.wait("terminal focus target", lambda: [
            n for n in self.nodes() if n.get("label") == expected and "focus" in n.get("actions", [])])
        self.call("act", dict(id=candidates[-1]["id"], action="focus"))
        self.wait("terminal focus identity", lambda: (self.focused() or {}).get("ID") == target["ID"])

    def command(self, command):
        self.text(command)
        self.key("Enter")

    def output(self, marker):
        return self.wait("terminal output " + marker.strip(), lambda: (self.focused() or {}).get("Text", "").find(marker) >= 0)

    def shell_ready(self):
        self.wait("remote shell prompt", lambda: (self.focused() or {}).get("Text", "").rstrip().endswith("$"))

    def close_tab(self, host="Docker Password"):
        # A tab's accessibility node is atomic: its nested close button is not
        # exposed separately. Click the actual close control: 8 DIP right padding
        # and a 16 DIP icon, as the current tab tokens specify.
        self.window_focused()
        self.call("idle", dict(timeout=5000))
        self.wait("SSH workspace before close", lambda: self.snapshot()["Section"] == 0)
        tab = self.wait("active tab", lambda: self.node(host, "tab"))
        bounds = tab["bounds"]
        before = self.snapshot()["Tabs"]
        self.input(dict(type="click", x=bounds["x"] + bounds["width"] - 16,
                        y=bounds["y"] + bounds["height"] / 2))
        self.wait("SSH close confirmation or removal", lambda: self.snapshot()["Tabs"] == before - 1
                  or self.node("关闭选项卡", "button"))
        if self.node("关闭选项卡", "button"):
            self.click("关闭选项卡")
        self.wait("closed tab", lambda: self.snapshot()["Tabs"] == before - 1)

    def check_grid(self):
        def settled():
            s = self.focused()
            return s if s and s["Columns"] == s["TerminalColumns"] and s["Rows"] == s["TerminalRows"] and s["Columns"] > 0 else None
        s = self.wait("terminal and SSH grid", settled)
        marker = "__GRID_" + str(self.counter) + "__"
        self.command("printf '\\n" + marker + "%s__END__\\n' \"$(stty size)\"")
        self.output("\n" + marker + str(s["Rows"]) + " " + str(s["Columns"]) + "__END__\n")
        return s["Columns"], s["Rows"]

    def run(self):
        password = (self.task_dir / "password").read_text().strip()
        passphrase = (self.task_dir / "key_passphrase").read_text().strip()
        initial = self.snapshot()
        if initial["CatalogVersion"] != 1 or len(initial["SSHProfiles"]) != 4 or len(initial["FileProfiles"]) != 4:
            raise RuntimeError("SQLite fixture did not load independent SSH and file sessions")
        print("PASS native UI: SQLite session catalog")
        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.wait("first host trust", lambda: self.node("信任并连接", "button"))
        self.click("取消")
        self.state(3)
        with sqlite3.connect(self.config_root / "Burrow/burrow.db") as db:
            if db.execute("SELECT count(*) FROM host_keys").fetchone()[0] != 0:
                raise RuntimeError("rejected host key was persisted")
        self.close_tab()
        print("PASS native UI: first host-key rejection")

        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.click("信任并连接")
        self.state(1)
        self.focus_terminal()
        self.command("printf '\\n%s\\n' BURROW_NATIVE_OK")
        self.output("\nBURROW_NATIVE_OK\n")
        self.check_grid()
        print("PASS native UI: password, trust, keyboard, remote PTY size")

        self.command("vim -Nu NONE -n")
        self.wait("vim alternate screen", lambda: any(marker in (self.focused() or {}).get("Text", "")
                  for marker in ("[No Name]", "VIM - Vi IMproved")))
        self.text("iUI_VIM_日本語")
        self.output("UI_VIM_日本語")
        self.key("Escape")
        self.text(":q!")
        self.key("Enter")
        self.shell_ready()
        self.command("printf '\\n%s\\n' BURROW_VIM_DONE")
        self.output("\nBURROW_VIM_DONE\n")
        self.command("top -d 1")
        self.wait("top alternate screen", lambda: "Tasks:" in (self.focused() or {}).get("Text", "") and "PID" in (self.focused() or {}).get("Text", ""))
        self.text("q")
        self.shell_ready()
        self.command("printf '\\n%s\\n' BURROW_TOP_DONE")
        self.output("\nBURROW_TOP_DONE\n")
        print("PASS native UI: vim Unicode input and top interactive exit")

        original_grid = self.check_grid()
        self.command("BURROW_UI_VARIABLE=alpha; printf '\\n%s\\n' BURROW_VARIABLE_SET")
        self.output("\nBURROW_VARIABLE_SET\n")
        connections_before_split = self.server_connections()
        if connections_before_split[1] != 1:
            raise RuntimeError("native fixture did not have exactly one SSH transport before splitting")
        self.key("Cmd+D")
        split_sessions = self.shared_shells(2)
        if len({(s["Connection"], s["Generation"]) for s in split_sessions}) != 1:
            raise RuntimeError("split shells did not retain the same connection instance")
        if self.server_connections() != connections_before_split:
            raise RuntimeError("splitting created another TCP connection or repeated authentication")
        print("PASS native UI: split reuses one TCP connection and authentication")
        default_marker = "BURROW_DEFAULT_SPLIT_" + str(self.counter)
        self.command("printf '\\n%s\\n' " + default_marker)
        def default_target():
            sessions = self.sessions()
            return sessions if any("\n" + default_marker + "\n" in s["Text"] for s in sessions) else None
        default_sessions = self.wait("default split keyboard target", default_target)
        new_id = max(s["ID"] for s in split_sessions)
        if ([s["ID"] for s in split_sessions if s["Focused"]] != [new_id]
                or [s["ID"] for s in default_sessions if s["Focused"]] != [new_id]
                or [s["ID"] for s in default_sessions if "\n" + default_marker + "\n" in s["Text"]] != [new_id]):
            raise RuntimeError("new split did not own the default keyboard focus")
        print("PASS native UI: new split owns default keyboard focus")
        self.focus_terminal(session_id=new_id)
        self.call("idle", dict(timeout=5000))
        self.command("printf '\\n__NEW_VAR__%s__END__\\n' \"${BURROW_UI_VARIABLE-unset}\"")
        self.output("\n__NEW_VAR__unset__END__\n")
        self.focus_terminal(session_id=min(s["ID"] for s in split_sessions))
        self.command("printf '\\n__OLD_VAR__%s__END__\\n' \"${BURROW_UI_VARIABLE-unset}\"")
        self.output("\n__OLD_VAR__alpha__END__\n")
        self.focus_terminal(session_id=new_id)
        split_grid = self.check_grid()
        if original_grid == split_grid:
            raise RuntimeError("split did not change the terminal grid")
        self.menu("Large window")
        self.wait("window resize grid", lambda: ((self.focused() or {}).get("Columns"), (self.focused() or {}).get("Rows")) != split_grid)
        self.check_grid()
        buttons = [n for n in self.nodes() if n.get("label") == "关闭窗格 Docker Password" and n.get("role") == "button"]
        if not buttons:
            raise RuntimeError("close pane button missing")
        button = max(buttons, key=lambda n: n["bounds"]["x"])
        self.call("act", dict(id=button["id"], action="press"))
        self.click("关闭窗格")
        self.shared_shells(1)
        if self.server_connections() != connections_before_split:
            raise RuntimeError("closing one pane changed the surviving shared transport")
        self.focus_terminal()
        self.check_grid()
        self.command("printf '\\n%s\\n' BURROW_BEFORE_RECONNECT")
        self.output("\nBURROW_BEFORE_RECONNECT\n")
        print("PASS native UI: Cmd+D, independent shell, resize and close pane")

        self.menu("Disconnect")
        self.state(0)
        self.menu("Reconnect")
        self.secret("输入 SSH 密码", password)
        self.state(1)
        self.focus_terminal()
        self.output("BURROW_BEFORE_RECONNECT")
        self.command("printf '\\n%s\\n' BURROW_RECONNECT_OK")
        self.output("\nBURROW_RECONNECT_OK\n")
        self.close_tab()
        print("PASS native UI: disconnect, reconnect, preserved history, close tab")

        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password + "-invalid")
        self.state(3)
        self.close_tab()
        print("PASS native UI: wrong password reports failure")

        self.catalog_isolation(password)

        for host, value in [("Docker Plain Key", ""), ("Docker Encrypted Key", passphrase)]:
            self.open_host(host)
            if value:
                self.secret("输入私钥口令", value)
            self.state(1)
            self.focus_terminal(host)
            self.command("printf '\\n%s\\n' BURROW_KEY_OK")
            self.output("\nBURROW_KEY_OK\n")
            self.close_tab(host)
        print("PASS native UI: plain and encrypted private keys")

        self.open_host("Docker Agent")
        self.state(1)
        self.focus_terminal("Docker Agent")
        self.command("printf '\\n%s\\n' BURROW_AGENT_OK")
        self.output("\nBURROW_AGENT_OK\n")
        print("PASS native UI: isolated SSH agent")
        self.menu("Quit")


@contextmanager
def native_window(repo, bundle, env, config_root, run_dir, phase, ready_label="Docker Password", ready_role="listItem", expect_crash=False):
    log_path = run_dir / ("native-app-" + str(phase) + ".log")
    status_path = config_root / "app-exit-status"
    status_path.write_text("pending")
    launch = ["/usr/bin/open", "-n", "-W", "-a", str(bundle), "--stdout", str(log_path), "--stderr", str(log_path)]
    for key in ("BURROW_INTEGRATION_CONFIG", "MYGO_AUTOMATION", "BURROW_TEST_DIR", "SSH_AUTH_SOCK"):
        launch += ["--env", key + "=" + env[key]]
    with log_path.open("w") as log:
        app = subprocess.Popen(launch, cwd=repo, env=env, stdout=log, stderr=log)
        try:
            end = time.monotonic() + 30
            connection = None
            while time.monotonic() < end:
                if app.poll() is not None:
                    raise RuntimeError("native integration application exited before connecting; log: " + str(log_path))
                try:
                    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                    connection.settimeout(20)
                    connection.connect(env["MYGO_AUTOMATION"])
                    break
                except (FileNotFoundError, ConnectionRefusedError):
                    connection.close()
                    connection = None
                    time.sleep(0.05)
            if connection is None:
                raise RuntimeError("native automation socket did not start; log: " + str(log_path))
            with connection:
                driver = Driver(connection, Path(env["BURROW_TEST_DIR"]), config_root)
                try:
                    driver.wait("native window", lambda: driver.call("windows").get("windows"))
                    driver.wait("native view", lambda: driver.node(ready_label, ready_role))
                    driver.menu("Focus window")
                    driver.wait("focused native window", lambda: any(w.get("focused") for w in driver.call("windows").get("windows", [])))
                    yield driver
                except Exception:
                    # Preserve labels and read-only state, never text-field values.
                    try:
                        diagnosis = dict(state=driver.snapshot(), nodes=[
                            {key: node[key] for key in ("id", "parent", "role", "label", "states", "bounds") if key in node}
                            for node in driver.nodes()])
                        path = run_dir / ("failure-" + str(phase) + ".json")
                        path.write_text(json.dumps(diagnosis, indent=2))
                        os.chmod(path, 0o600)
                        print("Native failure state: " + str(path), flush=True)
                    except Exception as diagnostic_error:
                        print("Native failure state unavailable: " + type(diagnostic_error).__name__, file=sys.stderr)
                    raise
                finally:
                    driver.reader.close()
            exit_code = app.wait(timeout=15)
            if expect_crash:
                if status_path.read_text() == "ok":
                    raise RuntimeError("expected crash completed as a normal shutdown")
            elif exit_code != 0 or status_path.read_text() != "ok":
                raise RuntimeError("native application did not complete cleanly; log: " + str(log_path))
            if "WARNING: DATA RACE" in log_path.read_text():
                raise RuntimeError("Go race detector reported a native integration race; log: " + str(log_path))
        finally:
            if app.poll() is None:
                pid_path = config_root / "app.pid"
                if pid_path.exists():
                    pid = int(pid_path.read_text())
                    process = subprocess.run(["ps", "-p", str(pid), "-o", "command="], text=True, capture_output=True)
                    if str(bundle) in process.stdout:
                        os.kill(pid, signal.SIGTERM)
                try:
                    app.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    app.terminate()
                    app.wait(timeout=5)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepare-only", action="store_true")
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--file-protocols", action="store_true", help="run only the SFTP/FTP native UI checks")
    modes.add_argument("--concurrent-split", action="store_true", help="run only simultaneous shared-shell requests")
    modes.add_argument("--core-e2e", action="store_true", help="run real UI transfers, drag lifecycle and restart recovery")
    modes.add_argument("--coverage", action="store_true", help="run gateways, monitoring, file operations, settings and catalog management")
    parser.add_argument("--scenario", action="append", help="with --coverage: run only the named scenario; repeatable")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    task_dir = Path(os.environ["BURROW_TEST_DIR"]).resolve()
    if not args.prepare_only:
        ensure_unlocked()
    overlay, config_root = prepare(repo, task_dir, os.environ["BURROW_TEST_ADDRESS"])
    if args.prepare_only:
        print(overlay)
        return
    run_dir = Path(tempfile.mkdtemp(prefix="native-run-", dir=task_dir))
    binary = run_dir / "burrow-native-integration"
    env = dict(os.environ, BURROW_INTEGRATION_CONFIG=str(config_root), MYGO_AUTOMATION=str(run_dir / "native.sock"))
    env.setdefault("GOCACHE", "/private/tmp/burrow-go-cache")
    subprocess.run(["go", "build", "-race", "-modfile", str(overlay.parent / "integration.mod"),
                    "-overlay", str(overlay), "-o", str(binary), "."], cwd=repo, env=env, check=True)
    # AppKit needs a proper bundle to register/focus a native window reliably.
    # Reuse only the packaging resources; the executable is the overlay build.
    bundle = run_dir / "Burrow Integration.app"
    shutil.copytree(repo / "build/darwin-arm64/Burrow.app", bundle)
    info_path = bundle / "Contents/Info.plist"
    info = plistlib.loads(info_path.read_bytes())
    info["CFBundleIdentifier"] += ".integration." + str(os.getpid())
    info["CFBundleName"] = "Burrow Integration"
    info_path.write_bytes(plistlib.dumps(info))
    launch_binary = bundle / "Contents/MacOS" / info["CFBundleExecutable"]
    shutil.copy2(binary, launch_binary)
    subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    mode = "coverage" if args.coverage else "core-e2e" if args.core_e2e else "file-protocols" if args.file_protocols else "concurrent-split" if args.concurrent_split else "terminal"
    report = dict(mode=mode, status="failed", phases=[], configuration=str(config_root), bundle=str(bundle))
    print("Native test bundle: " + str(bundle), flush=True)
    try:
        with isolated_agent(task_dir) as agent_socket:
            env["SSH_AUTH_SOCK"] = agent_socket
            recovery = None
            for phase in range(1, 3 if args.core_e2e or args.coverage else 2):
                # The second coverage phase starts locked, before the catalog is shown.
                # A locked application starts on the unlock form, before the catalog.
                locked = bool(args.coverage and phase == 2 and recovery.get("locked"))
                with native_window(repo, bundle, env, config_root, run_dir, phase,
                                   ready_label="解锁密码" if locked else "Docker Password",
                                   ready_role="textField" if locked else "listItem") as driver:
                    if args.core_e2e:
                        from ssh_e2e_scenarios import run_core, run_core_recovered
                        if phase == 1:
                            recovery = run_core(driver)
                        else:
                            run_core_recovered(driver, recovery)
                    elif args.coverage:
                        from ssh_e2e_coverage import run_coverage, run_coverage_restarted
                        if phase == 1:
                            recovery = run_coverage(driver, args.scenario)
                        else:
                            run_coverage_restarted(driver, recovery)
                    elif args.file_protocols:
                        driver.run_file_protocols()
                    elif args.concurrent_split:
                        driver.run_concurrent_split()
                    else:
                        driver.run()
                driver.wait("SSH transports released after application exit", lambda: driver.server_connections()[1] == 0, timeout=5)
                if args.file_protocols or args.core_e2e or args.coverage:
                    ftp_fixture = json.loads((task_dir / "ftp-server.json").read_text())
                    driver.wait("FTP transport released after application exit",
                                lambda: driver.established_connections(ftp_fixture["container"], 21) == 0, timeout=5)
                report["phases"].append(dict(phase=phase, status="passed", clean_exit=True, race_free=True, transports_released=True))
                print("PASS native UI: phase " + str(phase) + " exits cleanly and releases all transports", flush=True)
        report["status"] = "passed"
    except Exception as error:
        report["error"] = type(error).__name__ + ": " + str(error)
        raise
    finally:
        result_path = run_dir / "result.json"
        result_path.write_text(json.dumps(report, indent=2) + "\n")
        os.chmod(result_path, 0o600)
        print("Native test report: " + str(result_path), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print("FAIL native integration: " + str(error), file=sys.stderr)
        sys.exit(1)
