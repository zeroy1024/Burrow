#!/usr/bin/env python3
"""Drive the real native UI against the disposable Docker SSH fixture.

Requires SSHDESK_TEST_DIR and SSHDESK_TEST_ADDRESS. It uses Go overlays for
automation and configuration isolation; release sources and user data stay intact.
"""

import argparse
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
)

var dockerIntegrationApp *app

type dockerProfileInfo struct {
    ID, Port int
    Name, User, Address, Auth, Protocol string
}

type dockerGroupInfo struct {
    ID int
    Name string
    Hosts []hostID
    Collapsed bool
}

func dockerIntegrationGroups(catalog *catalogData) []dockerGroupInfo {
    groups := make([]dockerGroupInfo, 0, len(catalog.groups))
    for _, g := range catalog.groups {
        groups = append(groups, dockerGroupInfo{ID:g.id, Name:g.name, Hosts:g.hosts, Collapsed:g.collapsed})
    }
    return groups
}

func dockerIntegrationProfiles(catalog *catalogData) []dockerProfileInfo {
    profiles := make([]dockerProfileInfo, 0, len(catalog.hosts))
    for id, h := range catalog.hosts {
        profiles = append(profiles, dockerProfileInfo{ID:int(id), Port:h.port,
            Name:h.name, User:h.user, Address:h.address, Auth:h.auth, Protocol:h.protocol})
    }
    sort.Slice(profiles, func(i,j int) bool { return profiles[i].ID < profiles[j].ID })
    return profiles
}

func dockerIntegrationConfigDir() (string, error) {
    path := os.Getenv("SSHDESK_INTEGRATION_CONFIG")
    if !filepath.IsAbs(path) { return "", errors.New("isolated integration configuration is required") }
    return path, nil
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
    type fileEntryInfo struct { Name string; Directory bool; Size int64 }
    type fileSessionInfo struct {
        ID, Host, State, Connection, Generation int
        Selected, Temporary, Loading bool
        Name, Protocol, Path, Error string
        Entries []fileEntryInfo
    }
    out := struct {
        Sessions []sessionInfo
        FileSessions []fileSessionInfo
        SSHProfiles, FileProfiles []dockerProfileInfo
        SSHGroups, FileGroups []dockerGroupInfo
        Prompts, Tabs, FileTabs, Section, CatalogVersion, Transfers int
        LocalOpen bool
    }{Prompts: len(a.sshPrompts), Tabs: len(a.tabs), FileTabs:len(a.fileSessions), Section:int(a.section),
      Transfers:len(a.transfers.tasks), LocalOpen:a.files.localOpen,
      SSHProfiles:dockerIntegrationProfiles(&a.sshCatalog), FileProfiles:dockerIntegrationProfiles(&a.fileCatalog),
      SSHGroups:dockerIntegrationGroups(&a.sshCatalog), FileGroups:dockerIntegrationGroups(&a.fileCatalog)}
    configData, err := os.ReadFile(a.catalogConfig.path)
    if err != nil { panic(err) }
    var saved struct { Version int `json:"version"` }
    if err := json.Unmarshal(configData, &saved); err != nil { panic(err) }
    out.CatalogVersion = saved.Version
    for _, tab := range a.tabs {
        for _, s := range tab.root.sessions() {
            item := sessionInfo{ID:s.id, Host:int(s.host), State:int(s.state),
                Connection:s.connection.id, Generation:s.connection.generation, Columns:s.columns, Rows:s.rows,
                Focused:tab == a.currentTab() && tab.focused == s, Closing:s.closing,
                Name:s.endpoint.name, Address:s.endpoint.address}
            item.QueueLength, item.OpeningSession = len(s.connection.queue), -1
            if s.ssh != nil {
                item.ShellReturned = s.ssh.conn != nil
                item.RunCanceled = s.ssh.ctx.Err() != nil
            }
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
        item := fileSessionInfo{ID:s.id, Host:int(s.host), State:int(s.state), Selected:i==a.activeFile,
            Temporary:s.temporary, Loading:s.remote.loading, Name:s.endpoint.name, Protocol:s.endpoint.protocol,
            Path:s.remote.path, Error:s.remote.errorText}
        if s.errorText != "" { item.Error = s.errorText }
        if s.connection != nil { item.Connection, item.Generation = s.connection.id, s.connection.generation }
        for _, e := range s.remote.directories[s.remote.path] {
            item.Entries = append(item.Entries, fileEntryInfo{Name:e.name, Directory:e.dir, Size:e.size})
        }
        out.FileSessions = append(out.FileSessions, item)
    }
    data, err := json.Marshal(out)
    if err != nil { panic(err) }
    if err := os.WriteFile(filepath.Join(os.Getenv("SSHDESK_TEST_DIR"), "native-observation.json"), data, 0600); err != nil { panic(err) }
}

func init() {
    if err := os.WriteFile(filepath.Join(os.Getenv("SSHDESK_INTEGRATION_CONFIG"), "app.pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil { panic(err) }
    mygo.App.WhenReady(func() {
        mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Label:"Integration", Submenu:[]*mygo.MenuItem{
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
                if a:=dockerIntegrationApp; a!=nil { a.startEditHostDraft(0); a.window.Invalidate() }
            }},
            {Label:"Delete catalog fixture", Click:func(*mygo.MenuItem,*mygo.Window){
                if a:=dockerIntegrationApp; a!=nil {
                    a.page().draft = hostDraft{id:0, deleting:true, files:a.section==sectionFiles}
                    a.page().addHostOpen = true
                    a.window.Invalidate()
                }
            }},
            {Label:"Quit", Click:func(*mygo.MenuItem,*mygo.Window){ mygo.App.Quit() }},
        }}}))
    })
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
    config_dir = config_root / "SSH Desk"
    config_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
    hosts = []
    for i, (name, auth, key) in enumerate([
        ("Docker Password", "password", ""),
        ("Docker Plain Key", "key", str(task_dir / "key_plain")),
        ("Docker Encrypted Key", "key", str(task_dir / "key_encrypted")),
        ("Docker Agent", "agent", ""),
    ]):
        hosts.append(dict(id=i, group_id=0, order=i, name=name, user="sshdesk", address=host.strip("[]"),
                          port=int(port), os=0, system="Linux", auth=auth, key_path=key))
    (config_dir / "catalog.json").write_text(json.dumps(dict(
        groups=[dict(id=0, name="Docker integration", order=0, collapsed=False)], hosts=hosts)))
    os.chmod(config_dir / "catalog.json", 0o600)
    mapping = {}
    def replace_once(content, before, after):
        if content.count(before) != 1:
            raise RuntimeError("integration overlay source no longer matches; isolation was not applied")
        return content.replace(before, after, 1)

    for relative in ["internal/desktop/catalog_config.go", "internal/desktop/terminal_config.go", "internal/desktop/ssh_sessions.go"]:
        original = repo / relative
        content = replace_once(original.read_text(), "os.UserConfigDir()", "dockerIntegrationConfigDir()")
        if relative.endswith("ssh_sessions.go"):
            # This source uses os only for UserConfigDir; the overlay's helper replaces it.
            content = content.replace('\n\t"os"', "")
        replacement = overlay_dir / original.name
        replacement.write_text(content)
        mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/app.go"
    replacement = overlay_dir / "app.go"
    replacement.write_text(replace_once(original.read_text(),
        "func (a *app) view(c *ui.Context) {", "func (a *app) view(c *ui.Context) {\n dockerIntegrationApp = a"))
    mapping[str(original)] = str(replacement)
    original = repo / "internal/desktop/terminal.go"
    replacement = overlay_dir / "terminal.go"
    replacement.write_text(replace_once(original.read_text(),
        'Label(h.name + " SSH 终端").Opacity(opacity)',
        'Label(fmt.Sprintf("%s SSH 终端 #%d", h.name, session.id)).Opacity(opacity)'))
    mapping[str(original)] = str(replacement)
    original = repo / "main.go"
    replacement = overlay_dir / "main.go"
    content = replace_once(original.read_text(), "import (", 'import (\n "os"\n "path/filepath"\n _ "github.com/egoist/mygo/plugins/automation"')
    content = replace_once(content, "if err := desktop.Run(); err != nil {", '''err := desktop.Run()
    status := "ok"
    if err != nil { status = "error" }
    if writeErr := os.WriteFile(filepath.Join(os.Getenv("SSHDESK_INTEGRATION_CONFIG"), "app-exit-status"), []byte(status), 0600); writeErr != nil { log.Fatal(writeErr) }
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
        if fixture["address"] != os.environ["SSHDESK_TEST_ADDRESS"] or not fixture["container"].startswith("sshdesk-integration-"):
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
            if node.get("label") == label and (role is None or node.get("role") == role):
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

    def snapshot(self):
        self.menu("Snapshot")
        return json.loads((self.task_dir / "native-observation.json").read_text())

    def sessions(self):
        return self.snapshot().get("Sessions") or []

    def focused(self):
        return next((s for s in self.sessions() if s["Focused"]), None)

    def state(self, state, count=1):
        def check():
            if self.snapshot()["Section"] != 0 or self.node("打开文件会话", "button") is None:
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
            if state["Section"] != 0 or self.node("打开文件会话", "button") is None:
                return None
            return sessions if len(sessions) == count and all(s["State"] == 1 for s in sessions) else None
        return self.wait("shared SSH shells without authentication", check)

    def established_connections(self, container, port):
        command = "awk '$2 ~ /:" + format(port, "04X") + "$/ && $4 == \"01\" { n++ } END { print n+0 }' /proc/net/tcp /proc/net/tcp6"
        return int(subprocess.check_output(["docker", "exec", container, "sh", "-c", command], text=True).strip())

    def server_connections(self):
        # These observations are confined to the named disposable local fixture.
        logs = subprocess.check_output(["docker", "logs", self.container], stderr=subprocess.STDOUT, text=True)
        accepted = sum("Accepted " in line and " for sshdesk " in line for line in logs.splitlines())
        established = self.established_connections(self.container, 22)
        return accepted, established

    def fill(self, label, value):
        node = self.wait("field " + label, lambda: self.node(label, "textField"))
        self.call("act", dict(id=node["id"], action="setValue", text=value))

    def page(self, label, section):
        self.click(label)
        self.wait("selected page", lambda: self.snapshot()["Section"] == section)

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
            if tab is None or "checked" not in tab.get("states", []):
                return None
            names = {e["Name"] for e in selected["Entries"] or []}
            return selected if expected is None or set(expected).issubset(names) else None
        try:
            return self.wait("remote directory result", check)
        except RuntimeError:
            state = self.snapshot()
            selected = self.file()
            labels = {"远程文件", "打开 SSH 会话", "打开文件会话", selected["Name"] if selected else ""}
            print("Remote ready state: " + repr(dict(section=state["Section"], file=selected,
                  nodes=[(n.get("role"), n.get("label"), n.get("states"), n.get("bounds"))
                         for n in self.nodes() if n.get("label") in labels],
                  windows=[(w.get("focused"), w.get("visible")) for w in self.call("windows").get("windows", [])])), file=sys.stderr)
            raise

    def open_directory(self, name):
        try:
            item = self.wait("remote directory " + name, lambda: self.node(name, "listItem"))
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
        self.wait("file workspace before close", lambda: self.snapshot()["Section"] == 1 and self.node("打开 SSH 会话", "button") is not None)
        tab = self.wait("file tab", lambda: self.node(name, "tab"))
        bounds = tab["bounds"]
        before = self.snapshot()["FileTabs"]
        self.input(dict(type="click", x=bounds["x"] + bounds["width"] - 16,
                        y=bounds["y"] + bounds["height"] / 2))
        try:
            self.wait("closed file tab", lambda: self.snapshot()["FileTabs"] == before - 1)
        except RuntimeError:
            state = self.snapshot()
            print("File tab close state: " + repr(dict(before=before, after=state["FileTabs"], section=state["Section"],
                  ssh_tabs=state["Tabs"], bounds=bounds)), file=sys.stderr)
            raise

    def select_protocol(self, protocol):
        trigger = self.wait("protocol selector", lambda: next((n for n in self.nodes()
                            if n.get("value") == "SFTP" and "press" in n.get("actions", [])), None))
        self.call("act", dict(id=trigger["id"], action="focus"))
        self.key("Down")
        self.key("End" if protocol == "FTP" else "Home")
        self.key("Enter")

    def create_file_site(self, name, protocol, address):
        self.page("SFTP / FTP", 1)
        self.menu("Create catalog record")
        self.select_protocol(protocol)
        host, port = address.rsplit(":", 1)
        for label, value in (("名称", name), ("主机地址", host.strip("[]")), ("用户名", "sshdesk"), ("端口", port)):
            self.fill(label, value)
        self.click("添加")
        self.wait("saved file site", lambda: any(p["Name"] == name and p["Protocol"] == protocol.lower() for p in self.profiles(1)))

    def read_only_file_controls(self):
        for label in ("上传文件暂未开放", "新建文件夹暂未开放", "本地面板暂未开放"):
            node = self.wait("read-only control " + label, lambda: self.node(label))
            if "disabled" not in node.get("states", []):
                raise RuntimeError("a write or local-file control is available during read-only browsing")
        self.key("Cmd+Shift+L")
        state = self.snapshot()
        if state["LocalOpen"] or state["Transfers"]:
            raise RuntimeError("read-only file browsing opened local files or generated a transfer task")

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
        self.read_only_file_controls()
        return initial["Path"]

    def run_file_protocols(self):
        password = (self.task_dir / "password").read_text().strip()
        ftp_fixture = json.loads((self.task_dir / "ftp-server.json").read_text())
        if (not ftp_fixture["container"].startswith("sshdesk-ftp-integration-")
                or ftp_fixture["address"].rsplit(":", 1)[0] not in ("127.0.0.1", "localhost", "[::1]")):
            raise RuntimeError("FTP fixture metadata must identify the disposable local server")
        self.ftp_container = ftp_fixture["container"]
        saved_files = self.catalog_state(1)
        saved_ssh = self.catalog_state(0)

        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.click("信任并连接")
        source = self.state(1)[0]
        self.shell_ready()
        transport_before = self.server_connections()
        self.click("打开文件会话")
        files = self.files_ready({"hello.txt", "你好.txt", "nested", "denied"}, no_auth=True)
        if (not files["Temporary"] or files["Connection"] != source["Connection"]
                or self.server_connections() != transport_before or self.catalog_state(1) != saved_files):
            raise RuntimeError("SSH to SFTP did not reuse the source connection without changing saved sites")
        linked_file = files["ID"]
        self.browse_directories()
        self.click("打开 SSH 会话")
        self.wait("original SSH view", lambda: self.snapshot()["Section"] == 0 and (self.focused() or {}).get("ID") == source["ID"])
        self.click("打开文件会话")
        files = self.files_ready({"hello.txt", "nested"}, no_auth=True)
        if files["ID"] != linked_file or self.snapshot()["FileTabs"] != 1:
            raise RuntimeError("repeated SSH/SFTP shortcuts created duplicate target views")
        self.click("打开 SSH 会话")
        self.close_tab()
        self.page("SFTP / FTP", 1)
        self.click("刷新")
        self.files_ready({"hello.txt", "nested"}, no_auth=True)
        if self.server_connections() != transport_before:
            raise RuntimeError("closing every SSH pane disconnected or reauthenticated the surviving SFTP view")
        self.click("打开 SSH 会话")
        reopened = self.shared_shells(1)[0]
        if reopened["ID"] == source["ID"] or reopened["Connection"] != source["Connection"]:
            raise RuntimeError("SFTP did not open a fresh shell on its existing connection")
        self.shell_ready()
        self.command("printf '\\n%s\\n' SSHDESK_SFTP_SURVIVOR")
        self.output("\nSSHDESK_SFTP_SURVIVOR\n")
        self.click("打开文件会话")
        self.close_file_tab("Docker Password")
        self.page("SSH", 0)
        self.focus_terminal()
        self.command("printf '\\n%s\\n' SSHDESK_SHELL_SURVIVOR")
        self.output("\nSSHDESK_SHELL_SURVIVOR\n")
        self.close_tab()
        self.wait("last shared consumer released", lambda: self.server_connections()[1] == 0)
        print("PASS native files: SSH/SFTP shortcuts, one transport, independent close and read-only navigation")

        self.create_file_site("Docker Saved SFTP", "SFTP", os.environ["SSHDESK_TEST_ADDRESS"])
        if self.catalog_state(0) != saved_ssh:
            raise RuntimeError("saving an SFTP site added or changed SSH records")
        self.open_host("Docker Saved SFTP")
        standalone = self.files_ready({"hello.txt", "nested"}, no_auth=True)
        if standalone["Temporary"]:
            raise RuntimeError("a saved SFTP site opened as a temporary site")
        standalone_before = self.server_connections()
        self.click("打开 SSH 会话")
        shell = self.shared_shells(1)[0]
        if shell["Connection"] != standalone["Connection"] or self.catalog_state(0) != saved_ssh:
            raise RuntimeError("standalone SFTP to SSH created a new connection or saved host")
        self.shell_ready()
        self.command("printf '\\n%s\\n' SSHDESK_STANDALONE_SFTP")
        self.output("\nSSHDESK_STANDALONE_SFTP\n")
        if self.server_connections() != standalone_before:
            raise RuntimeError("opening SSH from a saved SFTP site repeated authentication")
        self.close_tab("Docker Saved SFTP")
        self.page("SFTP / FTP", 1)
        self.click("刷新")
        self.files_ready({"hello.txt", "nested"}, no_auth=True)
        self.close_file_tab("Docker Saved SFTP")
        self.wait("standalone SFTP released", lambda: self.server_connections()[1] == 0)
        print("PASS native files: independently saved SFTP site and temporary SSH view")

        self.page("SSH", 0)
        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        target = self.state(1)[0]
        self.shell_ready()
        ssh_before_ftp = self.server_connections()
        self.create_file_site("Docker FTP", "FTP", ftp_fixture["address"])
        if self.catalog_state(0) != saved_ssh:
            raise RuntimeError("saving an FTP site changed the SSH catalog")
        self.open_host("Docker FTP")
        self.secret("输入 FTP 密码", password + "-invalid")
        self.wait("wrong FTP password failure", lambda: (self.file() or {}).get("State") == 3)
        self.close_file_tab("Docker FTP")
        self.open_host("Docker FTP")
        self.secret("输入 FTP 密码", password)
        ftp = self.files_ready({"hello.txt", "你好.txt", "nested", "denied"})
        if ftp["Protocol"] != "ftp" or ftp["Connection"] != 0:
            raise RuntimeError("ordinary FTP was incorrectly attached to an SSH connection")
        self.browse_directories()
        self.click("打开 SSH 会话")
        profile = next(p for p in self.profiles(0) if p["Name"] == "Docker Password")
        choice = profile["Name"] + " · " + profile["User"] + "@" + profile["Address"] + ":" + str(profile["Port"])
        self.click(choice)
        self.wait("existing FTP-linked SSH view", lambda: self.snapshot()["Section"] == 0 and (self.focused() or {}).get("ID") == target["ID"])
        if self.snapshot()["Prompts"] or self.server_connections() != ssh_before_ftp:
            raise RuntimeError("FTP shortcut did not reuse the selected existing SSH view")
        self.click("打开文件会话")
        returned = self.files_ready({"hello.txt", "nested"})
        if returned["ID"] != ftp["ID"] or returned["Protocol"] != "ftp":
            raise RuntimeError("SSH shortcut did not return to its existing linked FTP view")
        self.page("SSH", 0)
        self.close_tab()
        self.page("SFTP / FTP", 1)
        self.click("刷新")
        self.files_ready({"hello.txt", "nested"})
        if self.server_connections()[1] != 0 or self.established_connections(self.ftp_container, 21) != 1:
            raise RuntimeError("FTP and SSH connection lifetimes were not independent")
        print("PASS native files: FTP login, permission errors, read-only browsing and explicit SSH target reuse")
        self.menu("Quit")

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
        return (state["SSHProfiles"], state["SSHGroups"]) if section == 0 else (state["FileProfiles"], state["FileGroups"])

    def add_catalog_record(self, name):
        self.menu("Create catalog record")
        host, port = os.environ["SSHDESK_TEST_ADDRESS"].rsplit(":", 1)
        for label, value in (("名称", name), ("主机地址", host.strip("[]")), ("用户名", "sshdesk"), ("端口", port)):
            self.fill(label, value)
        self.click("添加")

    def delete_catalog_fixture(self):
        self.menu("Delete catalog fixture")
        def button():
            return next((n for n in self.nodes() if n.get("role") == "button" and n.get("label") in ("删除主机", "删除站点")), None)
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
        self.add_catalog_record("Docker SSH Only")
        self.wait("saved SSH-only record", lambda: len(self.profiles(0)) == len(before_ssh) + 1)
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("adding an SSH host changed the file catalog")
        self.menu("Edit catalog fixture")
        self.fill("名称", "Docker SSH Renamed")
        self.fill("主机地址", "127.0.0.2")
        self.click("保存")
        self.wait("saved SSH edit", lambda: next(p for p in self.profiles(0) if p["ID"] == 0)["Name"] == "Docker SSH Renamed")
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("editing an SSH host changed the file catalog")
        original = next(p for p in before_ssh if p["ID"] == 0)
        if any(s["Name"] != original["Name"] or s["Address"] != original["Address"] for s in self.sessions()):
            raise RuntimeError("editing a catalog record changed an already connected session identity")
        self.delete_catalog_fixture()
        self.wait("deleted SSH record", lambda: all(p["ID"] != 0 for p in self.profiles(0)))
        self.state(1)
        if self.catalog_state(1) != before_file_catalog:
            raise RuntimeError("deleting an SSH host changed the file catalog")
        self.focus_terminal()
        self.command("printf '\\n%s\\n' SSHDESK_AFTER_CATALOG_DELETE")
        self.output("\nSSHDESK_AFTER_CATALOG_DELETE\n")
        self.close_tab()

        self.page("SFTP / FTP", 1)
        if self.snapshot()["FileTabs"]:
            raise RuntimeError("switching to files opened a file session without an explicit request")
        stable_ssh = self.catalog_state(0)
        self.add_catalog_record("Docker File Only")
        self.wait("saved file-only record", lambda: len(self.profiles(1)) == len(before_files) + 1)
        self.menu("Edit catalog fixture")
        self.fill("名称", "Docker File Renamed")
        self.click("保存")
        self.wait("saved file edit", lambda: next(p for p in self.profiles(1) if p["ID"] == 0)["Name"] == "Docker File Renamed")
        self.delete_catalog_fixture()
        self.wait("deleted file record", lambda: all(p["ID"] != 0 for p in self.profiles(1)))
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

    def focus_terminal(self, host="Docker Password"):
        target = self.focused()
        expected = host + " SSH 终端 #" + str(target["ID"]) if target else ""
        candidates = [n for n in self.nodes() if n.get("label") == expected and "focus" in n.get("actions", [])]
        if not candidates:
            raise RuntimeError("terminal focus target missing")
        self.call("act", dict(id=candidates[-1]["id"], action="focus"))

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
        self.wait("SSH workspace before close", lambda: self.node("打开文件会话", "button") is not None)
        tab = self.wait("active tab", lambda: self.node(host, "tab"))
        bounds = tab["bounds"]
        self.input(dict(type="click", x=bounds["x"] + bounds["width"] - 16,
                        y=bounds["y"] + bounds["height"] / 2))
        self.wait("closed tab", lambda: not self.sessions())

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
        if initial["CatalogVersion"] != 2 or len(initial["SSHProfiles"]) != 4 or len(initial["FileProfiles"]) != 4:
            raise RuntimeError("legacy fixture catalog did not migrate to two independent v2 catalogs")
        backup = json.loads((self.config_root / "SSH Desk/catalog.json.v1.bak").read_text())
        if len(backup.get("hosts", [])) != 4 or backup.get("version", 0) != 0:
            raise RuntimeError("legacy catalog backup was not preserved")
        print("PASS native UI: legacy catalog migration")
        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.wait("first host trust", lambda: self.node("信任并连接", "button"))
        self.click("取消")
        self.state(3)
        if (self.config_root / "SSH Desk/known_hosts").exists():
            raise RuntimeError("rejected host key was persisted")
        self.close_tab()
        print("PASS native UI: first host-key rejection")

        self.open_host("Docker Password")
        self.secret("输入 SSH 密码", password)
        self.click("信任并连接")
        self.state(1)
        self.focus_terminal()
        self.command("printf '\\n%s\\n' SSHDESK_NATIVE_OK")
        self.output("\nSSHDESK_NATIVE_OK\n")
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
        self.command("printf '\\n%s\\n' SSHDESK_VIM_DONE")
        self.output("\nSSHDESK_VIM_DONE\n")
        self.command("top -d 1")
        self.wait("top alternate screen", lambda: "Tasks:" in (self.focused() or {}).get("Text", "") and "PID" in (self.focused() or {}).get("Text", ""))
        self.text("q")
        self.shell_ready()
        self.command("printf '\\n%s\\n' SSHDESK_TOP_DONE")
        self.output("\nSSHDESK_TOP_DONE\n")
        print("PASS native UI: vim Unicode input and top interactive exit")

        original_grid = self.check_grid()
        self.command("SSHDESK_UI_VARIABLE=alpha; printf '\\n%s\\n' SSHDESK_VARIABLE_SET")
        self.output("\nSSHDESK_VARIABLE_SET\n")
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
        default_marker = "SSHDESK_DEFAULT_SPLIT_" + str(self.counter)
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
        self.key("Cmd+2")
        self.call("idle", dict(timeout=5000))
        self.command("printf '\\n__NEW_VAR__%s__END__\\n' \"${SSHDESK_UI_VARIABLE-unset}\"")
        self.output("\n__NEW_VAR__unset__END__\n")
        self.key("Cmd+1")
        self.command("printf '\\n__OLD_VAR__%s__END__\\n' \"${SSHDESK_UI_VARIABLE-unset}\"")
        self.output("\n__OLD_VAR__alpha__END__\n")
        self.key("Cmd+2")
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
        self.shared_shells(1)
        if self.server_connections() != connections_before_split:
            raise RuntimeError("closing one pane changed the surviving shared transport")
        self.focus_terminal()
        self.check_grid()
        self.command("printf '\\n%s\\n' SSHDESK_BEFORE_RECONNECT")
        self.output("\nSSHDESK_BEFORE_RECONNECT\n")
        print("PASS native UI: Cmd+D, independent shell, resize and close pane")

        self.menu("Disconnect")
        self.state(0)
        self.menu("Reconnect")
        self.secret("输入 SSH 密码", password)
        self.state(1)
        self.focus_terminal()
        self.output("SSHDESK_BEFORE_RECONNECT")
        self.command("printf '\\n%s\\n' SSHDESK_RECONNECT_OK")
        self.output("\nSSHDESK_RECONNECT_OK\n")
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
            self.secret("私钥口令（未加密可留空）", value)
            self.state(1)
            self.focus_terminal(host)
            self.command("printf '\\n%s\\n' SSHDESK_KEY_OK")
            self.output("\nSSHDESK_KEY_OK\n")
            self.close_tab(host)
        print("PASS native UI: plain and encrypted private keys")

        self.open_host("Docker Agent")
        self.state(1)
        self.focus_terminal("Docker Agent")
        self.command("printf '\\n%s\\n' SSHDESK_AGENT_OK")
        self.output("\nSSHDESK_AGENT_OK\n")
        print("PASS native UI: isolated SSH agent")
        self.menu("Quit")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepare-only", action="store_true")
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--file-protocols", action="store_true", help="run only the SFTP/FTP native UI checks")
    modes.add_argument("--concurrent-split", action="store_true", help="run only simultaneous shared-shell requests")
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[1]
    task_dir = Path(os.environ["SSHDESK_TEST_DIR"]).resolve()
    if not args.prepare_only:
        ensure_unlocked()
    overlay, config_root = prepare(repo, task_dir, os.environ["SSHDESK_TEST_ADDRESS"])
    if args.prepare_only:
        print(overlay)
        return
    binary = task_dir / "sshdesk-native-integration"
    env = dict(os.environ, SSHDESK_INTEGRATION_CONFIG=str(config_root), MYGO_AUTOMATION=str(task_dir / "native.sock"))
    env.setdefault("GOCACHE", "/private/tmp/sshdesk-go-cache")
    subprocess.run(["go", "build", "-race", "-overlay", str(overlay), "-o", str(binary), "."], cwd=repo, env=env, check=True)
    # AppKit needs a proper bundle to register/focus a native window reliably.
    # Reuse only the packaging resources; the executable is the overlay build.
    bundle = task_dir / ("SSH Desk Integration " + str(os.getpid()) + ".app")
    shutil.copytree(repo / "build/darwin-arm64/SSH Desk.app", bundle)
    info_path = bundle / "Contents/Info.plist"
    info = plistlib.loads(info_path.read_bytes())
    info["CFBundleIdentifier"] = "dev.sshdesk.integration." + str(os.getpid())
    info["CFBundleName"] = "SSH Desk Integration"
    info_path.write_bytes(plistlib.dumps(info))
    launch_binary = bundle / "Contents/MacOS" / info["CFBundleExecutable"]
    shutil.copy2(binary, launch_binary)
    subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    with isolated_agent(task_dir) as agent_socket, (task_dir / "native-app.log").open("w") as log:
        env["SSH_AUTH_SOCK"] = agent_socket
        launch = ["/usr/bin/open", "-n", "-W", "-a", str(bundle), "--stdout", str(task_dir / "native-app.log"),
                  "--stderr", str(task_dir / "native-app.log")]
        for key in ("SSHDESK_INTEGRATION_CONFIG", "MYGO_AUTOMATION", "SSHDESK_TEST_DIR", "SSH_AUTH_SOCK"):
            launch += ["--env", key + "=" + env[key]]
        app = subprocess.Popen(launch, cwd=repo, env=env, stdout=log, stderr=log)
        print("Native test bundle: " + str(bundle), flush=True)
        try:
            end = time.monotonic() + 30
            connection = None
            while time.monotonic() < end:
                if app.poll() is not None:
                    raise RuntimeError("native integration application exited before connecting")
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
                raise RuntimeError("native automation socket did not start")
            with connection:
                driver = Driver(connection, task_dir, config_root)
                driver.wait("native window", lambda: driver.call("windows").get("windows"))
                driver.wait("native view", lambda: driver.node("Docker Password", "listItem"))
                driver.menu("Focus window")
                def focused_window():
                    if any(w.get("focused") for w in driver.call("windows").get("windows", [])):
                        return True
                    driver.menu("Focus window")
                    return False
                driver.wait("focused native window", focused_window, timeout=60)
                if args.file_protocols:
                    driver.run_file_protocols()
                elif args.concurrent_split:
                    driver.run_concurrent_split()
                else:
                    driver.run()
            if app.wait(timeout=10) != 0:
                raise RuntimeError("native integration application returned an error")
            if (config_root / "app-exit-status").read_text() != "ok":
                raise RuntimeError("native application did not complete cleanly")
            native_log = (task_dir / "native-app.log").read_text()
            if "WARNING: DATA RACE" in native_log:
                raise RuntimeError("Go race detector reported a native integration race")
            driver.wait("SSH transports released after application exit", lambda: driver.server_connections()[1] == 0, timeout=5)
            if args.file_protocols:
                driver.wait("FTP transport released after application exit",
                            lambda: driver.established_connections(driver.ftp_container, 21) == 0, timeout=5)
            print("PASS native UI: last consumer and application exit release the transport")
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
                    app.wait()


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print("FAIL native integration: " + str(error), file=sys.stderr)
        sys.exit(1)
