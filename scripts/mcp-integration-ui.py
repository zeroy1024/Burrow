#!/usr/bin/env python3
"""Exercise MCP -> native Burrow -> disposable SSH/file servers and task queue.

Production UI sources remain unchanged. The existing native integration overlay
isolates data/credentials and provides test-only controller actions.
"""

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import http.client
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shutil
import signal
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import urllib.parse
import uuid


def load_native(repo):
    spec = importlib.util.spec_from_file_location("burrow_native_integration", repo / "scripts/ssh-integration-ui.py")
    native = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(native)
    return native


def write_private(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False))
    os.chmod(path, 0o600)


def seed_files(repo, task_dir, config_root):
    database = config_root / "Burrow/burrow.db"
    now = datetime.now(timezone.utc).isoformat()
    password = (task_dir / "password").read_text().strip()
    credentials = {}
    with sqlite3.connect(database) as db:
        preferences = json.loads(db.execute("SELECT value FROM settings WHERE key='preferences'").fetchone()[0])
        preferences.update(locale="zh-Hans", transferParallel=1, restoreSessions=True)
        db.execute("UPDATE settings SET value=? WHERE key='preferences'", (json.dumps(preferences),))
        group = db.execute("SELECT id FROM catalog_groups WHERE scope='files'").fetchone()[0]
        for index, (name, metadata_file, tls_mode) in enumerate([
            ("MCP FTP", "ftp-server.json", "none"),
            ("MCP FTPS Explicit", "ftps-server.json", "explicit"),
            ("MCP FTPS Implicit", "ftps-implicit-server.json", "implicit"),
            ("MCP FTPS Untrusted", "ftps-server.json", "explicit"),
        ], start=4):
            metadata = json.loads((task_dir / metadata_file).read_text())
            host, port = metadata["address"].rsplit(":", 1)
            if host != "127.0.0.1":
                raise RuntimeError("MCP integration server is not on loopback")
            profile, revision = str(uuid.uuid4()), str(uuid.uuid4())
            value = dict(id=profile, auth="password", protocol="ftp", name=name, user="sshdesk", host=host,
                         port=int(port), os=0, system="Linux", createdAt=now, updatedAt=now,
                         passwordSaved=True, passwordRevision=revision,
                         files=dict(encoding="UTF-8", ftpMode="passive", ftpTls=tls_mode))
            db.execute("INSERT INTO profiles VALUES(?,?,?,?,?,?)", (profile, group, index, None, None, json.dumps(value)))
            secret = json.dumps(dict(revision=revision, secret=password)).encode()
            credentials["dev.burrow.app.integration\x00session/" + profile + "/credential"] = base64.b64encode(secret).decode()
        # An init command must never run on the MCP exec connection.
        rows = db.execute("SELECT id,value FROM profiles WHERE value LIKE '%Docker Plain Key%'").fetchall()
        for profile, value in rows:
            value = json.loads(value)
            if value["protocol"] == "ssh":
                value["ssh"]["initCommand"] = "touch /home/sshdesk/mcp-init-command-must-not-run"
                db.execute("UPDATE profiles SET value=? WHERE id=?", (json.dumps(value), profile))
    write_private(config_root / "integration-secrets.json", credentials)


def prepare(repo, task_dir, native):
    address = json.loads((task_dir / "server.json").read_text())["address"]
    overlay, config_root = native.prepare(repo, task_dir, address)
    seed_files(repo, task_dir, config_root)
    mapping = json.loads(overlay.read_text())["Replace"]
    probe = Path(mapping[str(repo / "internal/desktop/zz_docker_integration_probe.go")])
    source = probe.read_text()
    before = 'Label:"Integration", Submenu:[]*mygo.MenuItem{'
    if source.count(before) != 1:
        raise RuntimeError("native MCP menu isolation seam changed")
    probe.write_text(source.replace(before, before + '\n mcpIntegrationMenu(),', 1))
    extra = overlay.parent / "mcp-integration-probe.go"
    extra.write_text((repo / "dev/mcp-integration/probe.go.tmpl").read_text())
    mapping[str(repo / "internal/desktop/zz_mcp_integration_probe.go")] = str(extra)
    original = repo / "internal/desktop/file_connections.go"
    source = original.read_text()
    before = "TLSMode: options.FTPTLS, Mode: options.FTPMode, Encoding: options.Encoding,"
    if source.count(before) != 1:
        raise RuntimeError("FTPS CA injection seam changed")
    replacement = overlay.parent / "file_connections.go"
    replacement.write_text(source.replace(before, before + "\n TLSConfig: mcpIntegrationTLSConfig(h.name),", 1))
    mapping[str(original)] = str(replacement)
    # Diagnostic overlay logs only verbs/reply codes, never paths, credentials,
    # command arguments or data bodies. It does not change FTP behavior.
    original = repo / "internal/fileclient/ftp.go"
    source = original.read_text().replace('"net/textproto"', '"net/textproto"\n "os"', 1)
    before = "return c.control.ReadResponse(expected)"
    if source.count(before) != 1:
        raise RuntimeError("FTP verb diagnostic seam changed")
    source = source.replace(before, '''code,message,err := c.control.ReadResponse(expected)
 fmt.Fprintf(os.Stderr,"MCP-FTP connection=%s verb=%s code=%d\\n",c.connection.LocalAddr(),strings.Fields(format)[0],code)
 return code,message,err''', 1)
    replacement = overlay.parent / "ftp-diagnostics.go"
    replacement.write_text(source)
    mapping[str(original)] = str(replacement)
    overlay.write_text(json.dumps(dict(Replace=mapping)))
    return overlay, config_root


def build(repo, task_dir, overlay, template):
    run_dir = Path(tempfile.mkdtemp(prefix="mcp-native-run-", dir=task_dir))
    binary = run_dir / "burrow-mcp-integration"
    env = dict(os.environ, GOCACHE="/private/tmp/sshdesk-go-build")
    subprocess.run(["go", "build", "-race", "-modfile", str(overlay.parent / "integration.mod"),
                    "-overlay", str(overlay), "-o", str(binary), "."], cwd=repo, env=env, check=True)
    bundle = run_dir / "Burrow MCP Integration.app"
    shutil.copytree(template, bundle)
    info_path = bundle / "Contents/Info.plist"
    info = plistlib.loads(info_path.read_bytes())
    info["CFBundleIdentifier"] += ".mcp.integration." + str(os.getpid())
    info["CFBundleName"] = "Burrow MCP Integration"
    info_path.write_bytes(plistlib.dumps(info))
    shutil.copy2(binary, bundle / "Contents/MacOS" / info["CFBundleExecutable"])
    subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True)
    return bundle, run_dir


class MCP:
    def __init__(self, state, role="full", protocol="2026-07-28"):
        self.endpoint = state["Endpoint"]
        self.key = state["Keys"][role]["Secret"]
        self.counter = 0
        self.protocol = protocol

    def request(self, method, params, timeout=150):
        self.counter += 1
        params = dict(params, _meta={
            "io.modelcontextprotocol/protocolVersion": self.protocol,
            "io.modelcontextprotocol/clientInfo": dict(name="burrow-native-e2e", version="1"),
            "io.modelcontextprotocol/clientCapabilities": {},
        })
        body = json.dumps(dict(jsonrpc="2.0", id=self.counter, method=method, params=params)).encode()
        request = urllib.request.Request(self.endpoint, body, {
            "Authorization": "Bearer " + self.key,
            "Content-Type": "application/json", "Accept": "application/json, text/event-stream",
            "MCP-Protocol-Version": self.protocol,
            "Mcp-Method": method,
            "Mcp-Name": params.get("name", ""),
        })
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                result = json.load(response)
        except urllib.error.HTTPError as error:
            return dict(http_error=error.code, message=error.read().decode("utf-8", "replace"))
        if "error" in result:
            raise RuntimeError("MCP protocol error: " + result["error"].get("message", "unknown"))
        return result.get("result", {})

    def call(self, name, arguments=None, expect_error=False):
        result = self.request("tools/call", dict(name=name, arguments=arguments or {}))
        if "http_error" in result:
            if result["http_error"] != 401:
                raise RuntimeError("MCP HTTP " + str(result["http_error"]) + ": " + result.get("message", ""))
            return result
        value = result.get("structuredContent")
        if value is None:
            value = json.loads(result["content"][0]["text"])
        if bool(result.get("isError")) != expect_error:
            code = value.get("error", {}).get("code", "no_error")
            message = value.get("error", {}).get("message", "")
            raise RuntimeError("MCP " + name + " unexpected result: " + code + " " + message)
        return value


class Suite:
    def __init__(self, driver, task_dir, config_root, report):
        self.driver, self.task_dir, self.config_root, self.report = driver, task_dir, config_root, report
        self.counter = 0
        self.executor = ThreadPoolExecutor(max_workers=4)
        self.state = None
        self.full = None

    def action(self, action, **values):
        identity = str(uuid.uuid4())
        write_private(self.task_dir / "mcp-action.json", dict(id=identity, action=action, **values))
        self.driver.menu("MCP integration action")
        def result():
            path = self.task_dir / "mcp-action-result.json"
            if not path.exists():
                return None
            value = json.loads(path.read_text())
            return value if value.get("id") == identity else None
        value = self.driver.wait("MCP action " + action, result, timeout=45)
        if not value["ok"]:
            raise RuntimeError("MCP action " + action + ": " + value["error"])

    def start(self):
        self.action("setup")
        self.state = json.loads((self.config_root / "mcp-state.json").read_text())
        self.full = MCP(self.state)

    def connection(self, name, protocol=None):
        return next(c for c in self.state["Connections"] if c["name"] == name and (protocol is None or c["protocol"] == protocol))

    def case(self, name):
        self.report["cases"].append(dict(name=name, status="passed"))
        print("PASS MCP E2E: " + name, flush=True)

    def remote(self, container, command):
        if not container.startswith(("sshdesk-integration-", "sshdesk-ftp-integration-", "sshdesk-ftps-")):
            raise RuntimeError("refusing non-fixture container")
        return subprocess.check_output(["docker", "exec", container, "sh", "-c", command], text=True).strip()

    def with_prompt(self, future, secret=None):
        deadline = time.monotonic() + 120
        while not future.done():
            if time.monotonic() > deadline:
                raise RuntimeError("MCP call did not finish while processing authentication")
            trust = self.driver.node("信任并连接", "button")
            if trust:
                try:
                    self.driver.call("act", dict(id=trust["id"], action="press"))
                except RuntimeError as error:
                    if str(error) != "act: no_element":
                        raise
            elif secret is not None and self.driver.node("连接", "button"):
                self.driver.text(secret)
                self.driver.click("连接")
            else:
                time.sleep(0.05)
        return future.result()

    def wait_task(self, task_id, state="completed", role="full", timeout=45):
        client = MCP(self.state, role)
        def ready():
            value = client.call("tasks_get", dict(task_id=task_id))["task"]
            if value["State"] in ("failed", "intercepted", "partial_intercepted", "cancelled") and state != value["State"]:
                raise RuntimeError("task ended as " + value["State"] + ": " + value.get("Error", ""))
            return value if value["State"] == state else None
        return self.driver.wait("MCP task " + state, ready, timeout=timeout)

    def basics(self):
        ssh = self.connection("Docker Plain Key", "ssh")
        future = self.executor.submit(self.full.call, "ssh_exec", dict(connection_id=ssh["id"], command="printf 'MCP-SSH-OK'; printf 'diagnostic' >&2; exit 0"))
        value = self.with_prompt(future)
        if value["stdout"] != "MCP-SSH-OK" or value["stderr"] != "diagnostic" or value["exit_code"] != 0:
            raise RuntimeError("SSH stdout/stderr/exit status mismatch")
        self.case("SSH real channel, first host trust, separate stderr with exit zero")
        value = self.full.call("ssh_exec", dict(connection_id=ssh["id"], command="printf 'seven'; exit 7"), expect_error=True)
        if value["exit_code"] != 7 or value["stdout"] != "seven":
            raise RuntimeError("SSH nonzero exit lost output/status")
        self.case("SSH nonzero exit preserves stdout and exact exit code")
        value = self.full.call("ssh_exec", dict(connection_id=ssh["id"], command="test ! -t 0 && test ! -e /home/sshdesk/mcp-init-command-must-not-run && printf isolated"))
        if value["stdout"] != "isolated":
            raise RuntimeError("MCP inherited PTY or terminal init command")
        self.case("SSH no PTY and no terminal initialization command")
        value = self.full.call("ssh_exec", dict(connection_id=ssh["id"], command="sleep 10", timeout_seconds=1), expect_error=True)
        if value["error"]["code"] != "timed_out" or value["exit_code"] is not None:
            raise RuntimeError("SSH timeout fabricated an exit code")
        if self.full.call("ssh_exec", dict(connection_id=ssh["id"], command="printf alive"))["stdout"] != "alive":
            raise RuntimeError("SSH timeout damaged the shared MCP transport")
        self.case("SSH timeout cancels only its channel and transport remains usable")

    def cancellation(self):
        for protocol in ("2026-07-28", "2025-06-18"):
            self.cancel_protocol(protocol)

    def cancel_protocol(self, protocol):
        ssh = self.connection("Docker Plain Key", "ssh")
        command = "sleep 8; printf client-disconnect-" + uuid.uuid4().hex
        try:
            MCP(self.state, protocol=protocol).request("tools/call", dict(name="ssh_exec", arguments=dict(connection_id=ssh["id"], command=command)), timeout=0.5)
        except (TimeoutError, socket.timeout, urllib.error.URLError):
            pass
        else:
            raise RuntimeError("synchronous command unexpectedly completed before disconnect")
        database = self.config_root / "Burrow/burrow.db"
        def cancelled():
            with sqlite3.connect("file:" + str(database) + "?mode=ro", uri=True) as db:
                row = db.execute("SELECT status FROM mcp_operations WHERE action='ssh_exec' AND json_extract(request,'$.command')=?", (command,)).fetchone()
                return row is not None and row[0] == "cancelled"
        self.driver.wait("HTTP disconnect cancels synchronous SSH", cancelled, timeout=4)
        self.case(protocol + " HTTP disconnect cancels synchronous SSH and records cancellation")

    def policies(self):
        ssh = self.connection("Docker Plain Key", "ssh")
        server = json.loads((self.task_dir / "server.json").read_text())
        policy_id = "integration-" + uuid.uuid4().hex
        marker = "/home/sshdesk/mcp-command-policy-" + uuid.uuid4().hex
        command = "touch " + marker
        policy = dict(ID=policy_id, Version=1, Name="MCP command integration", Category="ssh", Enabled=True,
                      AllKeys=True, ConnectionIDs=[ssh["id"]], CommandRules=[dict(Mode="exact", Pattern=command)])
        self.action("policy", policy=policy)
        denied = self.full.call("ssh_exec", dict(connection_id=ssh["id"], command=command), expect_error=True)
        if denied["error"]["code"] != "intercepted" or self.remote(server["container"], "test ! -e " + marker + " && printf denied") != "denied":
            raise RuntimeError("command policy allowed a real remote side effect")
        self.action("delete_policy", policy_id=policy_id)
        with sqlite3.connect(self.config_root / "Burrow/burrow.db") as db:
            hit = json.loads(db.execute("SELECT hits FROM mcp_operations WHERE id=?", (denied["operation_id"],)).fetchone()[0])[0]
            if hit["PolicyID"] != policy_id or hit["Version"] != 1:
                raise RuntimeError("deleting a policy changed its historical hit snapshot")
        self.case("command policy blocks actual SSH side effects; deleted rule snapshot remains auditable")

        sftp = self.connection("Docker Plain Key", "sftp")
        folder = "/home/sshdesk/mcp-protected-" + uuid.uuid4().hex
        self.remote(server["container"], "install -d -o sshdesk -g sshdesk " + folder)
        policy_id = "integration-" + uuid.uuid4().hex
        policy = dict(ID=policy_id, Version=1, Name="MCP file integration", Category="files", Enabled=True,
                      AllKeys=True, ConnectionIDs=[sftp["id"]], FileRules=[dict(Side="remote", Kind="dir", Path=folder,
                      IncludeSubdirs=True, DeniedActions=dict(write=True))])
        self.action("policy", policy=policy)
        target = folder + "/new.txt"
        self.full.call("files_write", dict(connection_id=sftp["id"], path=target, text="original", mode="create"))
        result = self.full.call("files_write", dict(connection_id=sftp["id"], path=target, text="must not overwrite", mode="replace"), expect_error=True)
        if result["error"]["code"] != "intercepted":
            raise RuntimeError("file write protection did not intercept actual replace")
        if self.full.call("files_read", dict(connection_id=sftp["id"], path=target))["text"] != "original":
            raise RuntimeError("file policy rejection changed the target")
        alias = "/home/sshdesk/mcp-alias-" + uuid.uuid4().hex
        self.remote(server["container"], "ln -s " + folder + " " + alias)
        alias_result = self.full.call("files_write", dict(connection_id=sftp["id"], path=alias + "/new.txt", text="alias overwrite", mode="replace"), expect_error=True)
        if alias_result["error"]["code"] != "intercepted":
            raise RuntimeError("SFTP alias bypassed the protected actual directory")
        self.action("delete_policy", policy_id=policy_id)
        self.case("SFTP create/write distinction, protected content unchanged and REALPATH alias interception")

        readonly = MCP(self.state, "readonly")
        readonly.call("files_write", dict(connection_id=sftp["id"], path=target, text="denied", mode="replace"), expect_error=True)
        outside = self.task_dir / "outside-authorized-root.txt"
        outside.write_text("outside")
        self.full.call("files_upload", dict(connection_id=sftp["id"], path="/home/sshdesk", local_path=str(outside), conflict="error"), expect_error=True)
        self.case("read-only key and local upload-root authorization reject real protocol mutations")

    def policy_during_read(self):
        connection = self.connection("MCP FTP")
        server = json.loads((self.task_dir / "ftp-server.json").read_text())
        filename = "mcp-slow-read-" + uuid.uuid4().hex + ".txt"
        self.remote(server["container"], "dd if=/dev/zero of=/home/sshdesk/" + filename + " bs=1048576 count=1 status=none; chown sshdesk:sshdesk /home/sshdesk/" + filename)
        future = self.executor.submit(self.full.call, "files_read", dict(connection_id=connection["id"], path="/" + filename, length=1048576), True)
        database = self.config_root / "Burrow/burrow.db"
        def reading():
            with sqlite3.connect("file:" + str(database) + "?mode=ro", uri=True) as db:
                return db.execute("SELECT count(*) FROM mcp_operations WHERE action='files_read' AND finished_at='' AND json_extract(request,'$.path')=?", ("/" + filename,)).fetchone()[0] > 0
        self.driver.wait("slow read accepted", reading)
        policy_id = "integration-" + uuid.uuid4().hex
        policy = dict(ID=policy_id, Version=1, Name="tighten while reading", Category="files", Enabled=True,
                      AllKeys=True, ConnectionIDs=[connection["id"]], FileRules=[dict(Side="remote", Kind="file", Path="/" + filename,
                      IncludeSubdirs=False, DeniedActions=dict(read=True))])
        self.action("policy", policy=policy)
        value = future.result(timeout=30)
        if "text" in value or value["error"]["code"] not in ("intercepted", "path_unverifiable"):
            raise RuntimeError("policy tightening leaked buffered content")
        self.action("delete_policy", policy_id=policy_id)
        self.case("policy tightened during real FTP read prevents buffered content delivery")

    def disconnected_transfer(self):
        source = self.large_source("dropped-response", 4 * 1024 * 1024)
        connection = self.connection("MCP FTP")
        endpoint = urllib.parse.urlparse(self.state["Endpoint"])
        transport = http.client.HTTPConnection(endpoint.hostname, endpoint.port, timeout=15)
        params = dict(name="files_upload", arguments=dict(connection_id=connection["id"], path="/", local_path=str(source), conflict="error"),
                      _meta={"io.modelcontextprotocol/protocolVersion":"2026-07-28", "io.modelcontextprotocol/clientInfo":dict(name="dropped-response",version="1"), "io.modelcontextprotocol/clientCapabilities":{}})
        body = json.dumps(dict(jsonrpc="2.0",id=1,method="tools/call",params=params)).encode()
        transport.request("POST",endpoint.path,body,headers={"Authorization":"Bearer "+self.state["Keys"]["full"]["Secret"],
            "Content-Type":"application/json","Accept":"application/json, text/event-stream","MCP-Protocol-Version":"2026-07-28","Mcp-Method":"tools/call","Mcp-Name":"files_upload"})
        database = self.config_root / "Burrow/burrow.db"
        def accepted():
            with sqlite3.connect("file:"+str(database)+"?mode=ro",uri=True) as db:
                row=db.execute("SELECT id,task_id FROM mcp_operations WHERE action='files_upload' AND json_extract(request,'$.local_path')=? AND task_id>0",(str(source),)).fetchone()
                return row
        operation,task_id=self.driver.wait("background acceptance before client decodes reply",accepted)
        transport.close()  # The client intentionally never decodes the task ID response.
        tasks=self.full.call("tasks_list")["tasks"]
        if not any(task["ID"]==task_id and task["OperationID"]==operation for task in tasks):
            raise RuntimeError("accepted task was not discoverable after response loss")
        self.wait_task(task_id)
        self.verify_uploaded(source)
        self.case("client closes before decoding task ID; own task list finds exactly the accepted transfer")

    def files(self):
        ssh_server = json.loads((self.task_dir / "server.json").read_text())
        protocols = [(self.connection("Docker Plain Key", "sftp"), ssh_server, "/home/sshdesk")]
        for name, filename in [("MCP FTP", "ftp-server.json"), ("MCP FTPS Explicit", "ftps-server.json"), ("MCP FTPS Implicit", "ftps-implicit-server.json")]:
            protocols.append((self.connection(name), json.loads((self.task_dir / filename).read_text()), "/"))
        for connection, server, remote_root in protocols:
            label = connection["name"]
            args = dict(connection_id=connection["id"], path=remote_root)
            entries = self.full.call("files_list", args)["entries"]
            if not any(entry["Name"] == "你好.txt" for entry in entries):
                raise RuntimeError(label + " Unicode listing missing")
            filename = remote_root.rstrip("/") + "/mcp-text-" + uuid.uuid4().hex + ".txt"
            content = "MCP 真实文本\nsecond line\n"
            self.full.call("files_write", dict(connection_id=connection["id"], path=filename, text=content, mode="create"))
            actual = self.full.call("files_read", dict(connection_id=connection["id"], path=filename))["text"]
            if actual != content:
                raise RuntimeError(label + " text round trip mismatch")
            self.full.call("files_write", dict(connection_id=connection["id"], path=filename, text="must not replace", mode="create"), expect_error=True)
            if self.full.call("files_read", dict(connection_id=connection["id"], path=filename))["text"] != content:
                raise RuntimeError(label + " create overwrote an existing target")
            self.full.call("files_write", dict(connection_id=connection["id"], path=filename, text="replaced", mode="replace"))
            physical = filename if remote_root != "/" else "/home/sshdesk" + filename
            digest = self.remote(server["container"], "sha256sum " + physical).split()[0]
            if digest != hashlib.sha256(b"replaced").hexdigest():
                raise RuntimeError(label + " independent server hash mismatch")
            self.case(label + " UTF-8 list/read, create/replace, collision and server SHA-256")
        untrusted = self.connection("MCP FTPS Untrusted")
        self.full.call("files_list", dict(connection_id=untrusted["id"], path="/"), expect_error=True)
        self.case("FTPS native adapter rejects untrusted certificate")

    def transfers(self):
        for name, protocol, root in [("Docker Plain Key", "sftp", "/home/sshdesk"), ("MCP FTP", None, "/"), ("MCP FTPS Explicit", None, "/"), ("MCP FTPS Implicit", None, "/")]:
            connection = self.connection(name, protocol)
            directory = self.task_dir / "mcp-local" / ("upload-" + uuid.uuid4().hex)
            directory.mkdir()
            (directory / "nested").mkdir()
            contents = {"hello.txt": b"mcp directory transfer\n", "nested/你好.txt": "目录内容\n".encode(), "empty": b""}
            for filename, body in contents.items():
                (directory / filename).write_bytes(body)
            queued = self.full.call("files_upload", dict(connection_id=connection["id"], path=root, local_path=str(directory), conflict="error"))
            self.wait_task(queued["task_id"], timeout=60)
            target = self.task_dir / "mcp-local" / ("download-" + uuid.uuid4().hex)
            target.mkdir()
            queued = self.full.call("files_download", dict(connection_id=connection["id"], path=root.rstrip("/") + "/" + directory.name, local_path=str(target), conflict="error"))
            self.wait_task(queued["task_id"], timeout=60)
            for filename, body in contents.items():
                actual = (target / directory.name / filename).read_bytes()
                if hashlib.sha256(actual).digest() != hashlib.sha256(body).digest():
                    raise RuntimeError(name + " directory download digest mismatch")
            self.case(name + " real queue directory upload/download, Unicode, nested and empty file")

    def close(self):
        self.executor.shutdown(wait=True)

    def database_task(self, task_id):
        database = self.config_root / "Burrow/burrow.db"
        with sqlite3.connect("file:" + str(database) + "?mode=ro", uri=True) as db:
            db.row_factory = sqlite3.Row
            row = db.execute("SELECT * FROM mcp_tasks WHERE id=?", (task_id,)).fetchone()
            return dict(row) if row is not None else None

    def wait_native_drained(self, task_id):
        def stopped():
            rows = self.driver.snapshot()["TransferTasks"]
            row = next((r for r in rows if r["ID"] == task_id), None)
            return row is None or not row["RunPresent"]
        self.driver.wait("MCP task worker stopped", stopped, timeout=30)

    def large_source(self, label, size=8 * 1024 * 1024):
        path = self.task_dir / "mcp-local" / (label + "-" + uuid.uuid4().hex + ".bin")
        block = bytes(range(256)) * 256
        with path.open("wb") as stream:
            for _ in range(size // len(block)):
                stream.write(block)
        return path

    def upload(self, source, role="full"):
        connection = self.connection("MCP FTP")
        return MCP(self.state, role).call("files_upload", dict(connection_id=connection["id"], path="/", local_path=str(source), conflict="error"))["task_id"]

    def wait_progress(self, task_id, role="full"):
        client = MCP(self.state, role)
        def progress():
            task = client.call("tasks_get", dict(task_id=task_id))["task"]
            if task["State"] in ("failed", "intercepted", "cancelled"):
                raise RuntimeError("transfer failed before progress: " + task.get("Error", ""))
            return task if task["State"] == "running" and task["Done"] > 0 else None
        return self.driver.wait("real FTP transfer progress", progress, timeout=30)

    def verify_uploaded(self, source):
        server = json.loads((self.task_dir / "ftp-server.json").read_text())
        remote_hash = self.remote(server["container"], "sha256sum /home/sshdesk/" + source.name).split()[0]
        if remote_hash != hashlib.sha256(source.read_bytes()).hexdigest():
            raise RuntimeError("resumed FTP transfer hash mismatch")

    def lifecycle(self):
        source = self.large_source("pause-resume")
        task_id = self.upload(source)
        self.wait_progress(task_id)
        self.action("show", task_id=task_id)
        self.driver.click("暂停")
        self.wait_task(task_id, "paused")
        self.wait_native_drained(task_id)
        paused = self.database_task(task_id)
        time.sleep(0.4)
        if self.database_task(task_id)["done"] != paused["done"]:
            raise RuntimeError("paused transfer kept advancing")
        self.driver.click("继续")
        self.wait_task(task_id, timeout=45)
        self.verify_uploaded(source)
        self.case("real MCP page pause/resume, stable paused offset and final SHA-256")

        running = self.upload(self.large_source("cancel-running"))
        self.wait_progress(running)
        queued_source = self.large_source("cancel-queued", 1024 * 1024)
        queued = self.upload(queued_source)
        if self.full.call("tasks_get", dict(task_id=queued))["task"]["State"] != "queued":
            raise RuntimeError("single-transfer concurrency did not leave the second task queued")
        self.full.call("tasks_cancel", dict(task_id=queued))
        self.wait_task(queued, "cancelled")
        self.full.call("tasks_cancel", dict(task_id=running))
        self.wait_task(running, "cancelled")
        self.wait_native_drained(running)
        server = json.loads((self.task_dir / "ftp-server.json").read_text())
        if self.remote(server["container"], "test ! -e /home/sshdesk/" + queued_source.name + " && printf absent") != "absent":
            raise RuntimeError("cancelled queued task created a remote file")
        self.case("queued/running cancellation through MCP and actual queue convergence")

        source = self.large_source("cancel-missing-source")
        detached = self.upload(source)
        self.wait_progress(detached)
        local_root = self.task_dir / "mcp-local"
        moved = self.task_dir / ("mcp-local-moved-" + uuid.uuid4().hex)
        local_root.rename(moved)
        try:
            self.full.call("tasks_cancel",dict(task_id=detached))
            self.wait_task(detached,"cancelled")
            self.driver.wait("remote staging cleaned without local source",lambda: not any(t["ID"]==detached for t in self.driver.snapshot()["TransferTasks"]),timeout=30)
        finally:
            moved.rename(local_root)
        self.case("cancel upload cleans remote staging after the local authorized source disappears")

        affected = self.upload(self.large_source("key-disable"), "lifecycle")
        self.wait_progress(affected, "lifecycle")
        pending = self.upload(self.large_source("key-disable-queued", 1024 * 1024), "lifecycle")
        other_source = self.large_source("other-key", 1024 * 1024)
        other = self.upload(other_source, "other")
        self.action("disable", role="lifecycle")
        self.driver.wait("disabled key tasks cancelled", lambda: all(self.database_task(t)["state"] == "cancelled" for t in (affected, pending)), timeout=30)
        if MCP(self.state, "lifecycle").call("tasks_list").get("http_error") != 401:
            raise RuntimeError("disabled key still authenticated")
        self.wait_task(other, role="other")
        self.action("enable", role="lifecycle")
        if any(self.database_task(t)["state"] != "cancelled" for t in (affected, pending)):
            raise RuntimeError("key re-enable restarted cancelled tasks")
        self.case("key disable cancels running/queued jobs, isolates other keys, enable does not restart")

        revoked = self.upload(self.large_source("key-revoke"), "lifecycle")
        self.wait_progress(revoked, "lifecycle")
        self.action("revoke", role="lifecycle")
        self.driver.wait("revoked task cancelled", lambda: self.database_task(revoked)["state"] == "cancelled", timeout=30)
        if MCP(self.state, "lifecycle").call("tasks_list").get("http_error") != 401:
            raise RuntimeError("revoked key still authenticated")
        self.case("key revoke converges the actual transfer and rejects subsequent authentication")

        restricted = self.upload(self.large_source("scope-shrink"),"scope")
        self.wait_progress(restricted,"scope")
        sftp = self.connection("Docker Plain Key","sftp")
        unrelated_source = self.large_source("scope-unrelated",1024*1024)
        unrelated = MCP(self.state,"scope").call("files_upload",dict(connection_id=sftp["id"],path="/home/sshdesk",local_path=str(unrelated_source),conflict="error"))["task_id"]
        self.action("shrink",role="scope",connection_id=self.connection("MCP FTP")["id"])
        self.driver.wait("only narrowed grant task cancelled",lambda:self.database_task(restricted)["state"]=="cancelled",timeout=30)
        self.wait_task(unrelated,role="scope")
        self.case("shrinking one connection grant cancels only affected work and preserves other grants")

        source = self.large_source("service-stop")
        stopped = self.upload(source)
        self.wait_progress(stopped)
        self.action("configure", enabled=False)
        self.driver.wait("service-stop pauses task", lambda: self.database_task(stopped)["state"] == "paused")
        self.wait_native_drained(stopped)
        self.action("configure", enabled=True)
        self.wait_task(stopped, "paused")
        self.action("resume", task_id=stopped)
        self.wait_task(stopped)
        self.verify_uploaded(source)
        self.case("service stop saves paused work; service start does not auto-resume; explicit resume succeeds")

    def recovery_prepare(self):
        source = self.large_source("application-restart", 12 * 1024 * 1024)
        server = json.loads((self.task_dir / "ftp-server.json").read_text())
        destination = "/home/sshdesk/" + source.name
        self.remote(server["container"], "printf 'previous contents' > " + destination +
                    " && chown sshdesk:sshdesk " + destination + " && chmod 600 " + destination)
        task_id = self.full.call("files_upload", dict(connection_id=self.connection("MCP FTP")["id"],
                                path="/", local_path=str(source), conflict="overwrite"))["task_id"]
        self.wait_progress(task_id)
        # Switch away from MCP while it is running, through the production rail.
        self.driver.click("SSH", role="tab")
        task = self.full.call("tasks_get", dict(task_id=task_id))["task"]
        if task["State"] != "running":
            raise RuntimeError("switching pages stopped the MCP task")
        self.driver.menu("Quit")
        return dict(task_id=task_id, source=str(source), destination=destination)

    def audit_failure(self):
        source = self.large_source("audit-failure")
        task_id = self.upload(source)
        self.wait_progress(task_id)
        database = self.config_root / "Burrow/burrow.db"
        with sqlite3.connect(database) as db:
            db.execute("CREATE TRIGGER mcp_e2e_audit_failure BEFORE INSERT ON mcp_operations BEGIN SELECT RAISE(FAIL,'integration audit storage failure'); END")
        try:
            ssh = self.connection("Docker Plain Key", "ssh")
            marker = "/home/sshdesk/audit-must-not-execute-" + uuid.uuid4().hex
            failure = self.full.call("ssh_exec",dict(connection_id=ssh["id"],command="touch " + marker),expect_error=True)
            if failure["error"]["code"] != "audit_unavailable":
                raise RuntimeError("audit failure was not exposed as unavailable")
            server = json.loads((self.task_dir / "server.json").read_text())
            if self.remote(server["container"],"test ! -e " + marker + " && printf absent") != "absent":
                raise RuntimeError("remote side effect occurred without durable beginning audit")
            control = self.full.call("tasks_cancel",dict(task_id=task_id))
            if control.get("audit_persisted") is not False or not control.get("cancel_requested"):
                raise RuntimeError("audit fault blocked cancellation or falsely claimed durable control audit")
            self.driver.wait("audit-fault task converges",lambda:self.database_task(task_id)["state"]=="cancelled",timeout=30)
        finally:
            with sqlite3.connect(database) as db:
                db.execute("DROP TRIGGER mcp_e2e_audit_failure")
        self.action("configure",enabled=True)
        self.case("audit storage failure prevents remote execution but still permits owned task cancellation")

    def recovery_finish(self, saved):
        self.start()
        task_id = saved["task_id"]
        task = self.full.call("tasks_get", dict(task_id=task_id))["task"]
        if task["State"] not in ("paused", "interrupted") or task["Done"] <= 0:
            raise RuntimeError("restart did not preserve an offline recoverable task")
        before = task["Done"]
        time.sleep(0.5)
        if self.full.call("tasks_get", dict(task_id=task_id))["task"]["Done"] != before:
            raise RuntimeError("restart silently resumed background work")
        server = json.loads((self.task_dir / "ftp-server.json").read_text())
        if self.remote(server["container"], "cat " + saved["destination"]) != "previous contents":
            raise RuntimeError("offline recovery replaced the protected destination before commit")
        self.action("resume", task_id=task_id)
        self.wait_task(task_id, timeout=60)
        self.verify_uploaded(Path(saved["source"]))
        self.case("normal restart preserves ownership, private FTP overwrite staging and original 0600 target; manual recovery matches SHA-256")

    def crash_prepare(self):
        source = self.large_source("crash-recovery", 12 * 1024 * 1024)
        task_id = self.upload(source)
        self.wait_progress(task_id)
        pid = int((self.config_root / "app.pid").read_text())
        command = subprocess.check_output(["ps","-p",str(pid),"-o","command="],text=True)
        if "Burrow MCP Integration.app/Contents/MacOS/" not in command:
            raise RuntimeError("refusing to terminate a process outside the isolated MCP test bundle")
        os.kill(pid,signal.SIGKILL)
        return dict(task_id=task_id,source=str(source))

    def crash_finish(self, saved):
        self.start()
        task_id = saved["task_id"]
        task = self.full.call("tasks_get",dict(task_id=task_id))["task"]
        if task["State"] not in ("paused","interrupted"):
            raise RuntimeError("crashed transfer was not loaded as offline/interrupted")
        self.action("resume",task_id=task_id)
        self.wait_task(task_id,timeout=60)
        self.verify_uploaded(Path(saved["source"]))
        self.case("forced process crash recovers owned checkpoint without replay; explicit resume matches SHA-256")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prepare-only", action="store_true")
    parser.add_argument("--reuse-run", type=Path, help="reuse a prior isolated native build/configuration while refining scenarios")
    parser.add_argument("--scenario", choices=("all", "lifecycle", "recovery"), default="all")
    parser.add_argument("--fixture", default=os.environ.get("SSHDESK_TEST_DIR"))
    parser.add_argument("--template", type=Path, help="existing Burrow.app resource template; defaults to the repository build")
    args = parser.parse_args()
    if not args.fixture:
        parser.error("--fixture or SSHDESK_TEST_DIR is required")
    repo = Path(__file__).resolve().parents[1]
    task_dir = Path(args.fixture).resolve()
    native = load_native(repo)
    native.ensure_unlocked()
    address = json.loads((task_dir / "server.json").read_text())["address"]
    os.environ.update(SSHDESK_TEST_DIR=str(task_dir), SSHDESK_TEST_ADDRESS=address)
    if args.reuse_run:
        previous = json.loads((args.reuse_run / "report.json").read_text())
        config_root, bundle = Path(previous["configuration"]), Path(previous["bundle"])
        run_dir = Path(tempfile.mkdtemp(prefix="mcp-native-retry-", dir=task_dir))
    else:
        overlay, config_root = prepare(repo, task_dir, native)
        if args.prepare_only:
            print(overlay)
            return
        bundle, run_dir = build(repo, task_dir, overlay, args.template or repo / "build/darwin-arm64/Burrow.app")
    report = dict(status="failed", scenario=args.scenario, cases=[], configuration=str(config_root), bundle=str(bundle))
    write_private(run_dir / "report.json", report)
    print("MCP native integration artifacts: " + str(run_dir), flush=True)
    try:
        with native.isolated_agent(task_dir) as agent:
            env = dict(os.environ, SSHDESK_INTEGRATION_CONFIG=str(config_root), SSH_AUTH_SOCK=agent,
                       MYGO_AUTOMATION=str(run_dir / "native.sock"))
            with native.native_window(repo, bundle, env, config_root, run_dir, 1, "主导航", "tabList") as driver:
                suite = Suite(driver, task_dir, config_root, report)
                try:
                    suite.start()
                    if args.scenario == "all":
                        suite.basics()
                        suite.cancellation()
                        suite.policies()
                        suite.policy_during_read()
                        suite.files()
                        suite.transfers()
                        suite.disconnected_transfer()
                    if args.scenario != "recovery":
                        suite.lifecycle()
                        suite.audit_failure()
                    recovery = suite.recovery_prepare()
                finally:
                    suite.close()
            with native.native_window(repo, bundle, env, config_root, run_dir, 2, "主导航", "tabList") as driver:
                suite = Suite(driver, task_dir, config_root, report)
                try:
                    suite.recovery_finish(recovery)
                    driver.menu("Quit")
                finally:
                    suite.close()
            with native.native_window(repo, bundle, env, config_root, run_dir, 3, "主导航", "tabList", expect_crash=True) as driver:
                suite = Suite(driver,task_dir,config_root,report)
                try:
                    suite.start()
                    crashed = suite.crash_prepare()
                finally:
                    suite.close()
            with native.native_window(repo,bundle,env,config_root,run_dir,4,"主导航","tabList") as driver:
                suite = Suite(driver,task_dir,config_root,report)
                try:
                    suite.crash_finish(crashed)
                    driver.menu("Quit")
                finally:
                    suite.close()
        report["status"] = "passed"
    except Exception as error:
        report["error"] = type(error).__name__ + ": " + str(error)
        raise
    finally:
        write_private(run_dir / "report.json", report)


if __name__ == "__main__":
    main()
