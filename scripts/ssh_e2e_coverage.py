"""Native coverage scenarios beyond the core set: gateways, monitoring, file
operations, settings and catalog management.

Mutations go through production forms, menus and keyboard input; the probe
snapshot and the Docker fixtures are only observed.
"""

import json
import re
import sqlite3
import subprocess
import time

from ssh_e2e_scenarios import (_authenticate, _catalog_open, _center, _check, _create_site, _drag_start, _file, _file_ready,
                               _fill, _node, _nodes, _pane_press, _press, _release, _row)


CONNECTED, DONE, FAILED = 1, 3, 4


def _fixture(driver):
    fixture = json.loads((driver.task_dir / "gateway-server.json").read_text())
    for name in ("jump", "socks"):
        _check(fixture[name]["address"].rsplit(":", 1)[0] == "127.0.0.1", "gateway fixtures must be loopback mappings")
    return fixture


def _choose(driver, label, option, role="popUpButton"):
    _press(driver, label, (role,))
    _press(driver, option, ("listItem", "menuItem"))


def _section(driver, label):
    driver.settled()
    _press(driver, label, ("tab",))
    driver.wait("selected section " + label,
                lambda: any("checked" in node.get("states", []) for node in _nodes(driver, label, ("tab",))))


def _add_ssh_host(driver, name, address, port, gateway=None, password=None):
    _section(driver, "SSH")
    before = len(driver.profiles(0))
    _press(driver, "添加", ("menuButton",))
    _press(driver, "添加主机", ("menuItem",))
    _node(driver, "新建 SSH 会话", ("dialog",))
    for label, value in (("名称", name), ("主机", address), ("端口", str(port)), ("用户名", "burrow")):
        _fill(driver, label, value)
    if password:
        _fill(driver, "密码", password)
    if gateway:
        _choose(driver, "连接网关", gateway)
    _press(driver, "仅保存")
    driver.wait("saved SSH host " + name, lambda: len(driver.profiles(0)) == before + 1
                and not _nodes(driver, "新建 SSH 会话", ("dialog",)))
    driver.settled()


def _save_gateway(driver, name):
    _press(driver, "保存")
    driver.wait("saved gateway " + name, lambda: not _nodes(driver, "新增网关", ("dialog",))
                and _nodes(driver, name, None))
    driver.settled()


def _gateway_form(driver, kind):
    # The empty page offers one button per kind; afterwards one form selects it.
    _section(driver, "网关")
    _press(driver, "新增网关" if _nodes(driver, "新增网关", ("button",)) else
           {"SSH": "添加 SSH 网关", "Socks5": "添加 SOCKS5 代理", "WireGuard": "添加 WireGuard"}[kind])
    _node(driver, "新增网关", ("dialog",))
    _press(driver, kind, ("radio",))
    driver.wait("gateway kind " + kind, lambda: any(
        "checked" in node.get("states", []) for node in _nodes(driver, kind, ("radio",))))


def _add_socks_gateway(driver, name, address, password, upstream=None):
    host, port = address.rsplit(":", 1)
    _gateway_form(driver, "Socks5")
    _fill(driver, "名称", name)
    _fill(driver, "SOCKS5 地址", host)
    _fill(driver, "SOCKS5 端口", port)
    _press(driver, "用户名密码", ("radio",))
    _fill(driver, "用户名", "burrow")
    _fill(driver, "密码", password)
    if upstream:
        _choose(driver, "上游网关", upstream)
    _save_gateway(driver, name)


def _add_ssh_gateway(driver, name, host):
    _gateway_form(driver, "SSH")
    _fill(driver, "名称", name)
    _choose(driver, "SSH 主机", host)
    _save_gateway(driver, name)


def _add_wireguard_gateway(driver, name, config):
    values = dict(re.findall(r"^(\w+) = (.+)$", open(config).read(), re.M))
    _gateway_form(driver, "WireGuard")
    _fill(driver, "名称", name)
    _fill(driver, "Peer 地址", values["Endpoint"])
    _fill(driver, "隧道地址", values["Address"])
    _fill(driver, "Peer 公钥", values["PublicKey"])
    _fill(driver, "WireGuard 私钥", values["PrivateKey"])
    _fill(driver, "Allowed IPs", values["AllowedIPs"])
    _save_gateway(driver, name)


def _peer(driver, host, expected):
    """Open the saved host and compare the client address its server saw."""
    _section(driver, "SSH")
    _catalog_open(driver, host)
    _authenticate(driver, lambda state: any(
        session["Name"] == host and session["State"] == CONNECTED for session in state.get("Sessions") or []))
    driver.shell_ready()
    driver.focus_terminal(host)
    driver.command("printf '\\nPEER=%s=END\\n' \"${SSH_CONNECTION%% *}\"")
    driver.output("PEER=" + expected + "=END")
    driver.close_tab(host)
    driver.wait("closed gateway session " + host, lambda: not driver.sessions())


def gateways(driver):
    fixture = _fixture(driver)
    password = (driver.task_dir / "password").read_text().strip()
    target = fixture["target"]["address"]
    jump_host, jump_port = fixture["jump"]["address"].rsplit(":", 1)

    _add_socks_gateway(driver, "E2E SOCKS", fixture["socks"]["address"], password)
    _press(driver, "测试连接")
    driver.wait("SOCKS5 gateway test result", lambda: any(
        node.get("label", "").startswith("连接成功") for node in driver.nodes() if node.get("role") == "status"))
    _node(driver, "SOCKS5 握手与认证成功", ("text",))
    _add_ssh_host(driver, "Via SOCKS", target, 22, gateway="E2E SOCKS", password=password)
    _peer(driver, "Via SOCKS", fixture["socks"]["internal"])
    print("PASS coverage: SOCKS5 gateway with password authentication reaches a bridge-only host", flush=True)

    _add_ssh_host(driver, "E2E Jump Host", jump_host, jump_port, password=password)
    _add_ssh_gateway(driver, "E2E Jump", "E2E Jump Host")
    _add_ssh_host(driver, "Via Jump", target, 22, gateway="E2E Jump", password=password)
    _peer(driver, "Via Jump", fixture["jump"]["internal"])
    print("PASS coverage: SSH jump gateway reaches a bridge-only host", flush=True)

    _add_wireguard_gateway(driver, "E2E WireGuard", fixture["wireguard"]["config"])
    _add_ssh_host(driver, "Via WireGuard", target, 22, gateway="E2E WireGuard", password=password)
    _peer(driver, "Via WireGuard", fixture["wireguard"]["internal"])
    print("PASS coverage: WireGuard gateway reaches a bridge-only host", flush=True)

    # The proxy's bridge address is reachable only from the jump host.
    _add_socks_gateway(driver, "E2E Chain", fixture["socks"]["internal"] + ":1080", password, upstream="E2E Jump")
    _add_ssh_host(driver, "Via Chain", target, 22, gateway="E2E Chain", password=password)
    _peer(driver, "Via Chain", fixture["socks"]["internal"])
    print("PASS coverage: SOCKS5 behind an SSH jump forms a two-hop route", flush=True)

    # A gateway whose proxy rejects its password fails its own test and, being
    # unreferenced, can be deleted; a referenced one says why it cannot.
    _add_socks_gateway(driver, "E2E Rejected", fixture["socks"]["address"], password + "-wrong")
    _press(driver, "测试连接")
    driver.wait("rejected SOCKS5 gateway test", lambda: _nodes(driver, "重新测试") and not any(
        node.get("label", "").startswith("连接成功") for node in driver.nodes() if node.get("role") == "status"))
    _press(driver, "更多网关操作", ("menuButton",))
    _press(driver, "删除网关…", ("menuItem",))
    _press(driver, "删除")
    driver.wait("deleted unreferenced gateway", lambda: not _nodes(driver, "E2E Rejected", ("text",)))
    driver.settled()
    x, y = _center(driver.wait("gateway row", lambda: next(
        (node for node in driver.nodes() if node.get("role") == "listItem"
         and node.get("label", "").startswith("E2E SOCKS，")), None)))
    driver.input(dict(type="click", x=x, y=y))
    _node(driver, "删除网关前需先解除全部引用", ("text",))
    print("PASS coverage: gateway self-test reports success and rejection; only unreferenced gateways are deletable",
          flush=True)

    # A file site copied from a gateway host keeps the gateway for SFTP.
    _section(driver, "SSH")
    before = len(driver.profiles(1))
    _press(driver, "更多操作 Via Jump", ("menuButton",))
    _press(driver, "快捷添加到 SFTP…", ("menuItem",))
    _node(driver, "新建 FTP/SFTP 会话", ("dialog",))
    _press(driver, "仅保存")
    driver.wait("saved SFTP site through the jump gateway", lambda: len(driver.profiles(1)) == before + 1
                and not _nodes(driver, "新建 FTP/SFTP 会话", ("dialog",)))
    driver.settled()
    _section(driver, "SFTP / FTP")
    _catalog_open(driver, "Via Jump")
    _authenticate(driver, lambda state: any(
        item["Name"] == "Via Jump" and item["State"] == CONNECTED and item["Selected"]
        for item in state.get("FileSessions") or []))
    _file_ready(driver, "Via Jump")
    driver.wait("bridge-only home listing over SFTP", lambda: {"hello.txt", "你好.txt"} <= _entries(driver))
    driver.close_file_tab("Via Jump")
    driver.wait("closed gateway file tab", lambda: not driver.snapshot()["FileTabs"])
    print("PASS coverage: an SFTP site added from a gateway host browses the bridge-only server", flush=True)

    _section(driver, "网关")
    _choose(driver, "应用默认出口", "E2E SOCKS")
    driver.settled()
    _add_ssh_host(driver, "Via Default", target, 22, password=password)
    _peer(driver, "Via Default", fixture["socks"]["internal"])
    _section(driver, "网关")
    _choose(driver, "应用默认出口", "直连")
    driver.settled()
    print("PASS coverage: the application default exit routes hosts that follow it", flush=True)


def _texts(driver):
    return [node.get("label", "") for node in driver.nodes() if node.get("role") in ("text", "status")]


def _connect(driver, host):
    _section(driver, "SSH")
    _catalog_open(driver, host)
    _authenticate(driver, lambda state: any(
        session["Name"] == host and session["State"] == CONNECTED for session in state.get("Sessions") or []))
    driver.shell_ready()


def _remote(container, *command):
    return subprocess.run(["docker", "exec", container, *command], check=True, capture_output=True, text=True).stdout.strip()


def monitoring(driver):
    container = json.loads((driver.task_dir / "server.json").read_text())["container"]
    _connect(driver, "Docker Password")
    _press(driver, "服务器监控", ("toggleButton",))
    hostname, kernel = _remote(container, "hostname"), _remote(container, "uname", "-r")
    cores = _remote(container, "nproc")

    def collected():
        texts = _texts(driver)
        meters = {node["label"]: node.get("value", "") for node in driver.nodes() if node.get("role") == "meter"}
        ready = (hostname in texts and kernel in texts and cores + " 核" in texts
                 and any(text.startswith("Ubuntu 24.04") for text in texts)
                 and re.fullmatch(r"[\d.]+ / [\d.]+ [KMGT]B", meters.get("内存", ""))
                 and re.fullmatch(r"[\d.]+ / [\d.]+ [KMGT]B", meters.get("磁盘", ""))
                 and any(re.fullmatch(r"\d+ 个进程 · CPU", text) for text in texts))
        return meters if ready else None
    meters = driver.wait("monitoring facts, memory, root disk and processes", collected, timeout=30)
    used, total = (float(value) for value in re.findall(r"[\d.]+", meters["内存"]))
    _check(0 < used <= total, "memory meter is inconsistent: " + meters["内存"])
    # A second reading turns counters into rates.
    driver.wait("CPU and network rates", lambda: (
        lambda texts: any(re.fullmatch(r"峰值 \d+%", text) for text in texts)
        and sum(bool(re.fullmatch(r"[\d.]+ [KMG]?B/s", text)) for text in texts) >= 2)(_texts(driver)), timeout=30)
    _press(driver, "服务器监控", ("toggleButton",))
    driver.wait("monitoring panel closed", lambda: "服务器状态" not in _texts(driver))
    driver.close_tab("Docker Password")
    driver.wait("closed monitored session", lambda: not driver.sessions())
    print("PASS coverage: server status shows live facts, CPU, memory, root disk, network and processes", flush=True)


def _entries(driver):
    return {entry["Name"] for entry in (_file(driver) or {}).get("Entries") or []}


def _remote_rows(driver):
    pane = _file(driver)["RemoteBounds"]
    return {node["label"] for node in driver.nodes() if node.get("role") == "row"
            and pane["x"] <= _center(node)[0] < pane["x"] + pane["width"]
            and pane["y"] < _center(node)[1] < pane["y"] + pane["height"]
            and node["label"] in _entries(driver)}


def _context(driver, name, item):
    x, y = _center(_row(driver, name, True))
    driver.input(dict(type="click", x=x, y=y, button="right"))
    _press(driver, item, ("menuItem",))


def _upload(driver, name, onto=None):
    """Drag a local row onto the remote pane, or onto one remote directory row."""
    driver.call("idle", dict(timeout=5000))
    start = _drag_start(driver, _row(driver, name, False))
    if onto:
        end = _center(_row(driver, onto, True))
    else:
        pane = _file(driver)["RemoteBounds"]
        end = pane["x"] + pane["width"] * 0.6, pane["y"] + pane["height"] * 0.8
    for step in range(1, 9):
        driver.input(dict(type="pointerMove", x=start[0] + (end[0] - start[0]) * step / 8,
                          y=start[1] + (end[1] - start[1]) * step / 8))
    _release(driver, end)


def _finished(driver, count):
    def check():
        tasks = driver.snapshot().get("TransferTasks") or []
        failed = [task["ErrorText"] for task in tasks if task["State"] == FAILED]
        _check(not failed, "transfer failed: " + "; ".join(failed))
        return len(tasks) == count and all(task["State"] == DONE for task in tasks)
    driver.wait(str(count) + " finished transfers", check, timeout=60)


def _file_operations(driver, site, container, home):
    root = "cov-" + driver.config_root.name + "-" + site.replace(" ", "-").lower()
    remote = home + "/" + root
    subprocess.run(["docker", "exec", container, "sh", "-c",
                    "mkdir -p '{0}/locked' && cd '{0}' && printf 'a\n' > a.txt && printf 'b\n' > b.txt"
                    " && printf 'remote same\n' > same.txt && chmod 000 locked && chown -R burrow:burrow ."
                    .format(remote)], check=True)
    local = driver.task_dir / "ui-files"
    for stale in local.iterdir():
        _check(stale.is_file(), "unexpected directory in the disposable local folder")
        stale.unlink()
    (local / "same.txt").write_text("local same\n")
    (local / "up.txt").write_text("upload after retry\n")

    def cat(name):
        return _remote(container, "cat", remote + "/" + name)

    def listing():
        return set(_remote(container, "ls", "-A", remote).split("\n"))

    _section(driver, "SFTP / FTP")
    if site in {profile["Name"] for profile in driver.profiles(1)}:
        _catalog_open(driver, site)
        _authenticate(driver, lambda state: any(
            item["Name"] == site and item["State"] == CONNECTED and item["Selected"]
            for item in state.get("FileSessions") or []))
        _file_ready(driver, site)
    else:
        # The FTP fixture confines the account to its home directory.
        _create_site(driver, site, "ftp", local, "/")
    x, y = _center(_row(driver, root, True))
    driver.input(dict(type="click", x=x, y=y, clicks=2))
    driver.wait("coverage directory listing", lambda: _entries(driver) == {"a.txt", "b.txt", "same.txt", "locked"})

    # Beside the local panel the remote search field collapses into a button.
    if not _nodes(driver, "搜索当前文件夹", ("textField",)):
        _pane_press(driver, "搜索当前文件夹")
    _fill(driver, "搜索当前文件夹", "b.t")
    driver.wait("search hides non-matching rows", lambda: _remote_rows(driver) == {"b.txt"})
    _press(driver, "清除搜索")
    driver.wait("cleared search restores rows",
                lambda: _remote_rows(driver) == {"a.txt", "b.txt", "same.txt", "locked"})

    _context(driver, "a.txt", "重命名")
    _node(driver, "重命名", ("dialog",))
    _fill(driver, "名称", "renamed-甲.txt")
    _press(driver, "保存")
    driver.wait("renamed entry", lambda: "renamed-甲.txt" in _entries(driver) and "a.txt" not in _entries(driver))
    _check(cat("renamed-甲.txt") == "a" and "a.txt" not in listing(), "rename did not move the server file")

    _pane_press(driver, "新建文件夹")
    _node(driver, "新建文件夹", ("dialog",))
    _fill(driver, "名称", "made-目录")
    _press(driver, "创建")
    driver.wait("created directory", lambda: "made-目录" in _entries(driver))
    _check(_remote(container, "sh", "-c", "test -d '" + remote + "/made-目录' && echo dir") == "dir",
           "new folder is missing on the server")

    _context(driver, "b.txt", "删除所选项目")
    _node(driver, "删除文件", ("dialog",))
    _press(driver, "删除")
    driver.wait("deleted entry", lambda: "b.txt" not in _entries(driver))
    _check("b.txt" not in listing(), "delete left the server file")

    if not driver.snapshot()["LocalOpen"]:
        _press(driver, "显示本地面板", ("toggleButton",))
    driver.wait("local panel", lambda: driver.snapshot()["LocalOpen"] and _nodes(driver, "up.txt", ("row",)))

    def conflict(choice):
        _upload(driver, "same.txt")
        _node(driver, "目标中已存在同名项目", ("dialog",))
        _press(driver, choice)
        driver.wait("conflict dialog dismissed", lambda: not _nodes(driver, "目标中已存在同名项目", ("dialog",)))

    # A skipped item is recorded as a finished task that moved no bytes.
    before = listing()
    conflict("跳过")
    _finished(driver, 1)
    skipped = driver.snapshot()["TransferTasks"][0]
    _check(cat("same.txt") == "remote same" and listing() == before and skipped["Transferred"] == 0,
           "skipping a name conflict still changed the server")
    conflict("保留两者")
    _finished(driver, 2)
    added = listing() - before
    _check(len(added) == 1 and cat("same.txt") == "remote same" and cat(added.pop()) == "local same",
           "keeping both did not create exactly one renamed copy")
    conflict("覆盖")
    _finished(driver, 3)
    _check(cat("same.txt") == "local same", "overwrite did not replace the server file")

    _upload(driver, "up.txt", onto="locked")
    failed = driver.wait("failed upload into an unreadable directory", lambda: next(
        (task for task in driver.snapshot().get("TransferTasks") or [] if task["State"] == FAILED), None), timeout=30)
    _check(failed["Name"] == "up.txt" and failed["ErrorText"], "the failed task carries no reason")
    _node(driver, "传输失败", ("text",))
    subprocess.run(["docker", "exec", container, "chmod", "755", remote + "/locked"], check=True)
    _press(driver, "重试传输")
    _finished(driver, 4)
    _check(cat("locked/up.txt") == "upload after retry", "retry did not deliver the file")

    _press(driver, "清除已完成")
    driver.wait("cleared finished transfers", lambda: not driver.snapshot().get("TransferTasks"))
    driver.close_file_tab(site)
    driver.wait("closed file tab", lambda: not driver.snapshot()["FileTabs"])
    print("PASS coverage: " + site + " search, rename, new folder, delete, conflict choices and failed-transfer retry",
          flush=True)


def file_operations(driver):
    ssh = json.loads((driver.task_dir / "server.json").read_text())
    ftp = json.loads((driver.task_dir / "ftp-server.json").read_text())
    _file_operations(driver, "Docker Password", ssh["container"], "/home/burrow")
    _file_operations(driver, "Coverage FTP", ftp["container"], "/home/burrow")


def _terminal(driver, host, marker):
    driver.focus_terminal(host)
    driver.command("printf '\\n%s\\n' " + marker)
    driver.output("\n" + marker + "\n")


def terminal_workspace(driver):
    container = json.loads((driver.task_dir / "server.json").read_text())["container"]
    _connect(driver, "Docker Password")
    _terminal(driver, "Docker Password", "COVERAGE_FIRST")

    # Hiding the sidebar widens the terminal; the remote PTY follows.
    columns = driver.focused()["Columns"]
    driver.key("Cmd+\\")
    _node(driver, "展开会话栏")
    driver.wait("wider terminal without the sidebar", lambda: driver.focused()["Columns"] > columns)
    driver.check_grid()
    driver.key("Cmd+\\")
    _node(driver, "收起会话栏")
    driver.wait("restored terminal width", lambda: driver.focused()["Columns"] == columns)

    # Tab shortcuts move between two hosts' tabs.
    _catalog_open(driver, "Docker Plain Key")
    driver.wait("second tab", lambda: driver.snapshot()["Tabs"] == 2
                and (driver.focused() or {}).get("Name") == "Docker Plain Key"
                and driver.focused()["State"] == CONNECTED)
    for chord, name in (("Cmd+1", "Docker Password"), ("Cmd+2", "Docker Plain Key"),
                        ("Cmd+[", "Docker Password"), ("Cmd+]", "Docker Plain Key")):
        driver.key(chord)
        driver.wait("tab shortcut " + chord, lambda: (driver.focused() or {}).get("Name") == name)

    # Closing with the shortcut asks first; the closed tab can be reopened.
    driver.key("Cmd+W")
    _press(driver, "关闭选项卡")
    driver.wait("closed tab", lambda: driver.snapshot()["Tabs"] == 1 and driver.snapshot()["ClosedTabs"] >= 1)
    driver.key("Cmd+Shift+T")
    driver.wait("reopened tab reconnects", lambda: driver.snapshot()["Tabs"] == 2 and any(
        session["Name"] == "Docker Plain Key" and session["State"] == CONNECTED for session in driver.sessions()))
    driver.close_tab("Docker Plain Key")

    # The server ends the session; the tab reports failure and reconnects on request.
    subprocess.run(["docker", "exec", container, "pkill", "-f", "sshd: burrow"], check=True)
    driver.wait("server-side disconnect shown", lambda: [session["State"] for session in driver.sessions()] == [FAILED - 1])
    _node(driver, "失败", ("status",))
    driver.key("Cmd+Shift+R")
    _authenticate(driver, lambda state: [session["State"] for session in state.get("Sessions") or []] == [CONNECTED])
    driver.shell_ready()
    _terminal(driver, "Docker Password", "COVERAGE_AFTER_RECONNECT")
    _check("COVERAGE_FIRST" in driver.focused()["Text"], "reconnecting discarded the earlier terminal history")
    driver.close_tab("Docker Password")
    driver.wait("closed terminal tabs", lambda: not driver.sessions())
    print("PASS coverage: sidebar toggle resize, tab shortcuts, reopen closed tab, server-side disconnect and reconnect",
          flush=True)


def key_management(driver):
    container = json.loads((driver.task_dir / "server.json").read_text())["container"]
    _settings_page(driver, "密钥与凭据")
    _press(driver, "生成…")
    _node(driver, "生成 SSH 密钥", ("dialog",))
    _fill(driver, "名称", "coverage-key")
    _press(driver, "生成")
    driver.wait("generated key listed", lambda: _nodes(driver, "coverage-key", ("text",))
                and not _nodes(driver, "生成 SSH 密钥", ("dialog",)))
    driver.settled()
    with sqlite3.connect("file:" + str(driver.config_root / "Burrow/burrow.db") + "?mode=ro", uri=True) as db:
        row = db.execute("SELECT * FROM managed_keys WHERE name='coverage-key'").fetchone()
    public = next(value for value in row if isinstance(value, (str, bytes))
                  and (value.decode() if isinstance(value, bytes) else value).startswith("ssh-ed25519 "))
    public = public.decode() if isinstance(public, bytes) else public
    # Only the generated public key crosses into the disposable fixture.
    subprocess.run(["docker", "exec", "-i", container, "sh", "-c", "cat >> /home/burrow/.ssh/authorized_keys"],
                   input=(public.strip() + "\n").encode(), check=True)

    _section(driver, "SSH")
    before = len(driver.profiles(0))
    host, port = json.loads((driver.task_dir / "server.json").read_text())["address"].rsplit(":", 1)
    _press(driver, "添加", ("menuButton",))
    _press(driver, "添加主机", ("menuItem",))
    _node(driver, "新建 SSH 会话", ("dialog",))
    for label, value in (("名称", "Generated Key"), ("主机", host), ("端口", port), ("用户名", "burrow")):
        _fill(driver, label, value)
    _press(driver, "私钥", ("radio",))
    _choose(driver, "私钥", "coverage-key")
    _press(driver, "仅保存")
    driver.wait("saved key host", lambda: len(driver.profiles(0)) == before + 1)
    driver.settled()
    _catalog_open(driver, "Generated Key")

    def authenticated():
        # Host-key trust may be asked; a credential must not be.
        _check(not _nodes(driver, "输入 SSH 密码", ("textField",)) and not _nodes(driver, "输入私钥口令", ("textField",)),
               "the generated key did not authenticate on its own")
        if _nodes(driver, "信任并连接", ("button",)):
            _press(driver, "信任并连接")
        return any(session["Name"] == "Generated Key" and session["State"] == CONNECTED
                   for session in driver.sessions())
    driver.wait("key authentication without a credential prompt", authenticated, timeout=40)
    driver.shell_ready()
    _terminal(driver, "Generated Key", "COVERAGE_GENERATED_KEY")
    driver.close_tab("Generated Key")
    driver.wait("closed key session", lambda: not driver.sessions())
    _settings_page(driver, "密钥与凭据")
    _node(driver, "被 1 个会话使用", ("text",))
    print("PASS coverage: a generated managed key authenticates a new host and reports its usage", flush=True)


def _preferences(driver):
    path = driver.config_root / "Burrow/burrow.db"
    with sqlite3.connect("file:" + str(path) + "?mode=ro", uri=True) as db:
        return json.loads(db.execute("SELECT value FROM settings WHERE key='preferences'").fetchone()[0])


def _checked(driver, label, role):
    return any("checked" in node.get("states", []) for node in _nodes(driver, label, (role,)))


def _settings_page(driver, label):
    _section(driver, "设置")
    _press(driver, label)
    driver.wait("settings page " + label, lambda: _checked(driver, label, "button"))


def catalog_management(driver):
    _section(driver, "SSH")

    def groups():
        return {group["Name"]: group["Hosts"] for group in driver.snapshot()["SSHGroups"]}

    def hosts():
        return {node["label"].split("，")[0] for node in driver.nodes() if node.get("role") == "listItem"
                and "，" in node.get("label", "")}

    def group_menu(group, item):
        # Rows settle after a drop; read the header position once layout is idle.
        driver.call("idle", dict(timeout=5000))
        x, y = _center(_node(driver, group))
        driver.input(dict(type="click", x=x, y=y, button="right"))
        _node(driver, "操作菜单", ("menu",))
        _press(driver, item, ("menuItem",))

    _press(driver, "添加", ("menuButton",))
    _press(driver, "新建分组", ("menuItem",))
    _node(driver, "新建分组", ("dialog",))
    _fill(driver, "分组名称", "Coverage 分组")
    _press(driver, "创建")
    driver.wait("created group", lambda: "Coverage 分组" in groups() and not _nodes(driver, "新建分组", ("dialog",)))
    driver.settled()

    group_menu("Coverage 分组", "修改组名")
    _fill(driver, "分组名称", "Coverage 改名")
    _press(driver, "保存")
    driver.wait("renamed group", lambda: "Coverage 改名" in groups() and "Coverage 分组" not in groups())
    driver.settled()

    everyone = {profile["Name"] for profile in driver.profiles(0)}
    _fill(driver, "搜索主机、地址或系统", "Agent")
    driver.wait("host search narrows the list", lambda: hosts() == {"Docker Agent"})
    _fill(driver, "搜索主机、地址或系统", "")
    driver.wait("cleared host search", lambda: hosts() == everyone)

    # Drag a host onto the new group's header, then back.
    def move(host, group):
        start = _drag_start(driver, _node(driver, host, ("listItem",)) if _nodes(driver, host, ("listItem",)) else
                            next(node for node in driver.nodes() if node.get("role") == "listItem"
                                 and node.get("label", "").startswith(host + "，")))
        end = _center(_node(driver, group))
        for step in range(1, 9):
            driver.input(dict(type="pointerMove", x=start[0] + (end[0] - start[0]) * step / 8,
                              y=start[1] + (end[1] - start[1]) * step / 8))
        _release(driver, end)
        driver.settled()

    agent = next(profile["ID"] for profile in driver.profiles(0) if profile["Name"] == "Docker Agent")
    move("Docker Agent", "Coverage 改名")
    driver.wait("host moved into the group", lambda: groups()["Coverage 改名"] == [agent])
    move("Docker Agent", "Docker integration")
    driver.wait("host moved back", lambda: groups()["Coverage 改名"] == [] and agent in groups()["Docker integration"])

    group_menu("Coverage 改名", "删除分组")
    _node(driver, "删除分组？", ("dialog",))
    _press(driver, "删除")
    driver.wait("deleted empty group", lambda: "Coverage 改名" not in groups())
    driver.settled()
    print("PASS coverage: catalog group create, rename, host search, drag between groups and delete", flush=True)


def settings(driver):
    _settings_page(driver, "通用")
    _choose(driver, "语言", "English")
    driver.wait("English interface", lambda: _nodes(driver, "Settings", ("tab",)) and _nodes(driver, "Gateways", ("tab",))
                and driver.snapshot()["Locale"] == "en" and _preferences(driver).get("locale") == "en")
    _press(driver, "Gateways", ("tab",))
    driver.wait("English gateway page", lambda: not any(
        re.search(r"[一-鿿]", node.get("label", "")) for node in driver.nodes()
        if node.get("role") in ("button", "tab", "popUpButton", "status")
        and not node.get("label", "").startswith(("E2E ", "Via "))))
    _press(driver, "Settings", ("tab",))
    _press(driver, "General")
    _choose(driver, "Language", "简体中文")
    driver.wait("Chinese interface restored", lambda: _nodes(driver, "设置", ("tab",))
                and _preferences(driver).get("locale") == "zh-Hans")

    _settings_page(driver, "外观")
    _press(driver, "深色", ("radio",))
    _press(driver, "紫色")
    driver.wait("dark theme and accent saved", lambda: _checked(driver, "深色", "radio") and _checked(driver, "紫色", "button")
                and _preferences(driver)["appTheme"] == "dark" and _preferences(driver)["accent"] != "#0a84ff")

    _settings_page(driver, "终端")
    before = _preferences(driver)
    _press(driver, "固定一种", ("radio",))
    _press(driver, "Dracula")
    _press(driver, "▏竖线", ("radio",))
    driver.wait("terminal scheme and cursor saved", lambda: (
        lambda now: now["termMode"] != before["termMode"] and "Dracula" in json.dumps(now)
        and now["term"]["CursorShape"] != before["term"]["CursorShape"])(_preferences(driver)))

    _settings_page(driver, "文件传输")
    before = _preferences(driver)
    _press(driver, "重命名", ("radio",))
    driver.wait("conflict policy saved", lambda: _checked(driver, "重命名", "radio") and _preferences(driver) != before)

    # Rebind the terminal-mode shortcut; the old chord must stop working.
    _settings_page(driver, "快捷键")
    # The row is scrolled into view first; press again if that scroll moved it.
    def recording():
        if driver.snapshot()["SettingsRecording"] == "mode.terminal":
            return True
        driver.call("idle", dict(timeout=5000))
        _press(driver, "终端模式 快捷键")
        time.sleep(0.3)
        return False
    driver.wait("recording the terminal-mode shortcut", recording)
    driver.key("Cmd+Alt+9")
    _node(driver, "恢复默认：终端模式")
    _section(driver, "SFTP / FTP")
    driver.key("Cmd+Alt+1")
    time.sleep(1)
    _check(driver.snapshot()["Section"] == 1, "the replaced shortcut still switched to the terminal page")
    driver.key("Cmd+Alt+9")
    driver.wait("rebound shortcut switches to the terminal page", lambda: driver.snapshot()["Section"] == 0)
    print("PASS coverage: language switch, appearance, terminal, transfer policy and shortcut rebinding are applied and saved",
          flush=True)
    return dict(settings=True)


UNLOCK = "coverage-unlock-" + "1"  # disposable phrase for the isolated configuration only


def application_lock(driver):
    _settings_page(driver, "密钥与凭据")
    _press(driver, "打开应用时需要解锁", ("switch",))
    _node(driver, "设置解锁密码", ("dialog",))
    # Reveal both fields so every typed value is read back before saving; a
    # hidden field cannot be verified and a lost keystroke would only show as
    # a save that does nothing. The phrase is disposable.
    def revealed():
        # Revealing one field can move the other's button; locate each afresh.
        hidden = _nodes(driver, "显示密码", ("button",))
        if hidden:
            x, y = _center(hidden[0])
            driver.input(dict(type="click", x=x, y=y))
            driver.call("idle", dict(timeout=5000))
        return not hidden
    driver.wait("revealed unlock fields", revealed)
    _fill(driver, "新密码", UNLOCK)
    _fill(driver, "确认密码", UNLOCK + "-mismatch")
    _press(driver, "保存")
    time.sleep(1)
    _check(_nodes(driver, "设置解锁密码", ("dialog",)) and not _checked(driver, "打开应用时需要解锁", "switch"),
           "mismatched confirmation enabled the application lock")
    _fill(driver, "确认密码", UNLOCK)
    # Correcting the field removes the mismatch message and moves the buttons.
    driver.wait("mismatch message cleared", lambda: not [
        node for node in driver.nodes() if node.get("role") == "status"])
    driver.call("idle", dict(timeout=5000))
    _press(driver, "保存")
    driver.wait("application lock enabled", lambda: not _nodes(driver, "设置解锁密码", ("dialog",))
                and _checked(driver, "打开应用时需要解锁", "switch") and _nodes(driver, "修改密码…"))
    driver.settled()
    print("PASS coverage: application lock requires a matching confirmation and is enabled", flush=True)
    return dict(locked=True)


SCENARIOS = dict(gateways=gateways, monitoring=monitoring, file_operations=file_operations,
                 catalog_management=catalog_management, terminal_workspace=terminal_workspace,
                 key_management=key_management, settings=settings, application_lock=application_lock)


def _released(driver):
    # A stream inside the userspace WireGuard tunnel cannot be closed once the
    # process is gone, so the running application must release every transport.
    driver.wait("the application released its SSH transports before quitting",
                lambda: driver.server_connections()[1] == 0, timeout=30)


def run_coverage(driver, only=None):
    """Run the scenarios, quit normally and return what the restarted application must still show."""
    checkpoint = dict(scenarios=list(only or SCENARIOS))
    for name in checkpoint["scenarios"]:
        checkpoint.update(SCENARIOS[name](driver) or {})
    # The restarted application reopens the last page; its launcher waits for the SSH catalog.
    _section(driver, "SSH")
    _released(driver)
    driver.menu("Quit")
    return checkpoint


def run_coverage_restarted(driver, checkpoint):
    if checkpoint.get("locked"):
        _node(driver, "解锁密码", ("textField",))
        _check(not _nodes(driver, "主导航", ("tabList",)), "the locked application exposes the workspace")
        _fill(driver, "解锁密码", UNLOCK + "-wrong")
        _press(driver, "解锁")
        _node(driver, "无法解锁，请检查密码或凭据数据", ("status",))
        _check(not _nodes(driver, "主导航", ("tabList",)), "a wrong password unlocked the application")
        _fill(driver, "解锁密码", UNLOCK)
        _press(driver, "解锁")
        driver.wait("unlocked workspace", lambda: _nodes(driver, "主导航", ("tabList",)))
        print("PASS coverage: restart stays locked on a wrong password and opens on the right one", flush=True)
    if "gateways" in checkpoint["scenarios"]:
        fixture = _fixture(driver)
        # Saved gateway and host passwords come back from the vault after restart.
        _peer(driver, "Via Chain", fixture["socks"]["internal"])
        _peer(driver, "Via WireGuard", fixture["wireguard"]["internal"])
        print("PASS coverage: saved gateways, routes and vault credentials survive a restart", flush=True)
    if checkpoint.get("settings"):
        preferences = _preferences(driver)
        _check(preferences["appTheme"] == "dark" and preferences["termMode"] != "follow"
               and preferences["locale"] == "zh-Hans", "saved preferences changed across the restart")
        _settings_page(driver, "外观")
        _check(_checked(driver, "深色", "radio") and _checked(driver, "紫色", "button"),
               "appearance settings were not restored")
        _section(driver, "SFTP / FTP")
        driver.key("Cmd+Alt+9")
        driver.wait("rebound shortcut after restart", lambda: driver.snapshot()["Section"] == 0)
        _settings_page(driver, "快捷键")
        _press(driver, "全部恢复默认")
        driver.wait("shortcuts restored", lambda: not _nodes(driver, "恢复默认：终端模式"))
        _section(driver, "SFTP / FTP")
        driver.key("Cmd+Alt+1")
        driver.wait("default shortcut works again", lambda: driver.snapshot()["Section"] == 0)
        print("PASS coverage: appearance, terminal and shortcut settings survive a restart; defaults can be restored",
              flush=True)
    if checkpoint.get("locked"):
        _settings_page(driver, "密钥与凭据")
        _press(driver, "打开应用时需要解锁", ("switch",))
        _node(driver, "关闭应用解锁", ("dialog",))
        _fill(driver, "当前密码", UNLOCK)
        _press(driver, "保存")
        driver.wait("application lock disabled", lambda: not _nodes(driver, "关闭应用解锁", ("dialog",))
                    and not _checked(driver, "打开应用时需要解锁", "switch"))
        driver.settled()
        print("PASS coverage: application lock is removed with the current password", flush=True)
    _released(driver)
    driver.menu("Quit")

