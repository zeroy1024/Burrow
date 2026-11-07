"""Native core scenarios; production input drives mutations, probes only observe.

run_core returns a JSON checkpoint after requesting a normal application quit.
The launcher restarts the same isolated configuration, then calls
run_core_recovered with a fresh Driver and that checkpoint.
"""

import hashlib
import json
from pathlib import Path
import subprocess
import time


CONNECTED, PAUSED, DONE, FAILED = 1, 2, 3, 4


def _check(condition, message):
    if not condition:
        raise RuntimeError(message)


def _nodes(driver, label, roles=None):
    return [node for node in driver.nodes()
            if node.get("label") == label
            and (roles is None or node.get("role") in roles)
            and "disabled" not in node.get("states", [])]


def _node(driver, label, roles=("button",)):
    return driver.wait("visible control " + label,
                       lambda: next(iter(_nodes(driver, label, roles)), None))


def _center(node):
    bounds = node["bounds"]
    _check(bounds["width"] > 0 and bounds["height"] > 0,
           "control has no visible bounds: " + node.get("label", ""))
    return bounds["x"] + bounds["width"] / 2, bounds["y"] + bounds["height"] / 2


def _press(driver, label, roles=("button",)):
    node = _node(driver, label, roles)
    if "offscreen" in node.get("states", []):
        driver.call("act", dict(id=node["id"], action="scrollIntoView"))
        driver.call("idle", dict(timeout=5000))
        node = _node(driver, label, roles)
    x, y = _center(node)
    driver.input(dict(type="click", x=x, y=y))


def _fill(driver, label, value):
    # Focus is an accessibility action; edits still pass through native input.
    node = _node(driver, label, ("textField",))
    driver.call("act", dict(id=node["id"], action="focus"))
    driver.wait("focused field " + label,
                lambda: str(driver.call("tree").get("focus")) == str(node["id"]))
    driver.key("Cmd+A")
    driver.key("Backspace")
    if value:
        driver.text(value)
    if "password" not in node.get("states", []):
        driver.wait("edited field " + label, lambda:
                    any(field.get("value", "") == value
                        for field in _nodes(driver, label, ("textField",))))


def _file(driver, name=None):
    return next((item for item in driver.snapshot().get("FileSessions") or []
                 if item["Selected"] and (name is None or item["Name"] == name)), None)


def _file_ready(driver, name):
    return driver.wait("connected file tab " + name, lambda: (
        item if (item := _file(driver, name)) and item["State"] == CONNECTED
        and not item["Loading"] and not item["LocalLoading"]
        and not item["Error"] and not item["LocalError"] else None), timeout=40)


def _select_file_tab(driver, session_id):
    sessions = driver.snapshot()["FileSessions"]
    index = next(i for i, item in enumerate(sessions) if item["ID"] == session_id)
    _check(index < 4, "core fixture exceeded the four numbered tab shortcuts")
    driver.key("Cmd+" + str(index + 1))
    driver.wait("selected file tab identity", lambda: (_file(driver) or {}).get("ID") == session_id)


def _authenticate(driver, ready):
    password = (driver.task_dir / "password").read_text().strip()
    end = time.monotonic() + 40
    while time.monotonic() < end:
        state = driver.snapshot()
        if ready(state):
            return state
        if _nodes(driver, "信任并连接", ("button",)):
            _press(driver, "信任并连接")
        else:
            prompt = next((label for label in ("输入 SSH 密码", "输入 FTP 密码")
                           if _nodes(driver, label, ("textField",))), None)
            if prompt:
                _fill(driver, prompt, password)
                _press(driver, "连接")
                driver.wait("authentication input accepted", lambda:
                            not _nodes(driver, prompt, ("textField",)))
        time.sleep(0.05)
    raise RuntimeError("native authentication did not establish the requested session")


def _catalog_open(driver, name):
    row = driver.wait("catalog row " + name, lambda: next((node for node in driver.nodes()
        if node.get("role") == "listItem" and
        (node.get("label") == name or node.get("label", "").startswith(name + "，"))), None))
    x, y = _center(row)
    driver.input(dict(type="click", x=x, y=y, clicks=2))


def _tasks(driver):
    return driver.snapshot().get("TransferTasks") or []


def _task(driver, name, predicate, timeout=60):
    def matching():
        candidates = [task for task in _tasks(driver) if task["Name"] == name]
        for task in candidates:
            _check(task["State"] != FAILED, "transfer failed: " + name + ": " + task["ErrorText"])
            if predicate(task):
                return task
        return None
    return driver.wait("transfer state " + name, matching, timeout=timeout)


def _row(driver, name, remote):
    def locate():
        session = _file(driver)
        if not session:
            return None
        pane = session["RemoteBounds" if remote else "LocalBounds"]
        for role in ("row", "listItem"):
            for node in _nodes(driver, name, (role,)):
                bounds = node["bounds"]
                left, top = max(bounds["x"], pane["x"]), max(bounds["y"], pane["y"])
                right = min(bounds["x"] + bounds["width"], pane["x"] + pane["width"])
                bottom = min(bounds["y"] + bounds["height"], pane["y"] + pane["height"])
                if right > left and bottom > top:
                    return dict(node, bounds=dict(x=left, y=top, width=right-left, height=bottom-top))
        return None
    return driver.wait("file row " + name, locate)


def _drag_start(driver, node):
    x, y = _center(node)
    driver.input(dict(type="pointerMove", x=x, y=y),
                 dict(type="pointerDown", x=x, y=y),
                 dict(type="pointerMove", x=x + 12, y=y + 8))
    return x + 12, y + 8


def _release(driver, point):
    driver.input(dict(type="pointerUp", x=point[0], y=point[1]))


def _transfer(driver, name, source_remote):
    _file_ready(driver, _file(driver)["Name"])
    driver.call("idle", dict(timeout=5000))
    row = _row(driver, name, source_remote)
    target = _file(driver)["LocalBounds" if source_remote else "RemoteBounds"]
    start = _drag_start(driver, row)
    end = target["x"] + target["width"] * 0.6, target["y"] + target["height"] * 0.65
    for step in range(1, 9):
        driver.input(dict(type="pointerMove", x=start[0] + (end[0] - start[0]) * step / 8,
                          y=start[1] + (end[1] - start[1]) * step / 8))
    _release(driver, end)
    return _task(driver, name, lambda task: task["SourceRemote"] == source_remote)


def _fixture(driver, protocol):
    filename = "server.json" if protocol == "sftp" else "ftp-server.json"
    fixture = json.loads((driver.task_dir / filename).read_text())
    prefix = "burrow-integration-" if protocol == "sftp" else "burrow-ftp-integration-"
    _check(fixture["container"].startswith(prefix), "unexpected Docker fixture identity")
    _check(fixture["address"].rsplit(":", 1)[0] in ("127.0.0.1", "localhost", "[::1]"),
           "core scenarios require a loopback fixture")
    fixture["root"] = "/home/burrow/e2e-" + driver.config_root.name
    return fixture


def _docker(fixture, *arguments, **kwargs):
    return subprocess.run(["docker", "exec", fixture["container"], *arguments],
                          check=True, capture_output=True, **kwargs)


def _hash(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _remote_hash(fixture, name):
    return _docker(fixture, "sha256sum", fixture["root"] + "/" + name,
                   text=True).stdout.split()[0]


def _prepare_data(driver, local):
    local.mkdir(mode=0o700, parents=True, exist_ok=True)
    generated = driver.task_dir / "core-generated"
    generated.mkdir(mode=0o700, exist_ok=True)
    expected = {}
    for protocol in ("sftp", "ftp"):
        fixture = _fixture(driver, protocol)
        _docker(fixture, "install", "-d", "-o", "burrow", "-g", "burrow", "-m", "0755",
                fixture["root"])
        upload = "core-" + protocol + "-upload.txt"
        download = "core-" + protocol + "-download.txt"
        (local / upload).write_bytes(("native upload " + protocol + "\n").encode() * 4096)
        payload = generated / download
        payload.write_bytes(("native download " + protocol + "\n").encode() * 4096)
        subprocess.run(["docker", "cp", str(payload),
                        fixture["container"] + ":" + fixture["root"] + "/" + download],
                       check=True, capture_output=True)
        expected[upload], expected[download] = _hash(local / upload), _hash(payload)
    large = generated / "large.bin"
    with large.open("wb") as stream:
        block = bytes(range(256)) * 4096
        for _ in range(32):
            stream.write(block)
    ftp = _fixture(driver, "ftp")
    for name in ("core-resume.bin", "core-cancel.bin", "core-restart.bin"):
        subprocess.run(["docker", "cp", str(large),
                        ftp["container"] + ":" + ftp["root"] + "/" + name],
                       check=True, capture_output=True)
        expected[name] = _hash(large)
    for protocol in ("sftp", "ftp"):
        fixture = _fixture(driver, protocol)
        _docker(fixture, "chown", "-R", "burrow:burrow", fixture["root"])
    return expected


def _create_site(driver, name, protocol, local, remote_directory=None):
    driver.key("Cmd+Alt+2")
    driver.wait("file page before creating site", lambda:
                driver.snapshot()["Section"] == 1 and not driver.snapshot()["SettingsOpen"])
    driver.key("Cmd+N")
    _node(driver, "新建 FTP/SFTP 会话", ("dialog",))
    _node(driver, "名称", ("textField",))
    if protocol == "ftp":
        _press(driver, "FTP", ("radioButton", "radio", "button"))
        driver.wait("FTP protocol selected", lambda:
                    any("checked" in node.get("states", [])
                        for node in _nodes(driver, "FTP", ("radio", "radioButton"))))
    fixture = _fixture(driver, protocol)
    host, port = fixture["address"].rsplit(":", 1)
    for label, value in (("名称", name), ("主机", host.strip("[]")),
                         ("用户名", "burrow"), ("端口", port)):
        _fill(driver, label, value)
    if not _nodes(driver, "远程目录", ("textField",)):
        _press(driver, "高级", ("disclosure",))
    driver.wait("advanced file fields expanded", lambda:
                bool(_nodes(driver, "远程目录", ("textField",))))
    remote = remote_directory
    if remote is None:
        remote = fixture["root"] if protocol == "sftp" else "/" + Path(fixture["root"]).name
    _fill(driver, "远程目录", remote)
    _fill(driver, "本地目录", str(local))
    if protocol == "ftp":
        _press(driver, "无", ("radioButton", "radio", "button"))
    _press(driver, "保存并连接")
    _authenticate(driver, lambda state: any(item["Name"] == name and item["State"] == CONNECTED
                  for item in state.get("FileSessions") or []))
    if not driver.snapshot()["LocalOpen"]:
        driver.key("Cmd+Shift+L")
    driver.call("idle", dict(timeout=5000))
    return _file_ready(driver, name)


def _ssh_lifecycle(driver):
    driver.key("Cmd+Alt+1")
    _catalog_open(driver, "Docker Password")
    _authenticate(driver, lambda state: len(state.get("Sessions") or []) == 1 and
                  state["Sessions"][0]["State"] == CONNECTED)
    driver.focus_terminal()
    driver.command("CORE_E2E_VALUE=original; printf '\\nCORE_E2E_ORIGINAL_READY\\n'")
    driver.output("\nCORE_E2E_ORIGINAL_READY\n")
    original = driver.focused()
    driver.key("Cmd+D")
    shells = driver.wait("independent split shells", lambda:
        items if len(items := driver.sessions()) == 2 and
        all(item["State"] == CONNECTED for item in items) else None)
    _check(len({item["Connection"] for item in shells}) == 1, "split did not share its transport")
    driver.command("printf '\\nCORE_E2E_NEW_%s\\n' \"${CORE_E2E_VALUE-unset}\"")
    driver.output("\nCORE_E2E_NEW_unset\n")
    closes = _nodes(driver, "关闭窗格 Docker Password", ("button",))
    close = max(closes, key=lambda node: node["bounds"]["x"])
    x, y = _center(close)
    driver.input(dict(type="click", x=x, y=y))
    _press(driver, "关闭窗格")
    survivor = driver.wait("surviving original shell", lambda:
        items[0] if len(items := driver.sessions()) == 1 else None)
    _check(survivor["ID"] == original["ID"] and survivor["Connection"] == original["Connection"],
           "closing a split replaced the surviving session")
    driver.focus_terminal()
    driver.command("printf '\\nCORE_E2E_OLD_%s\\n' \"$CORE_E2E_VALUE\"")
    driver.output("\nCORE_E2E_OLD_original\n")
    tab = _node(driver, "Docker Password", ("tab",))
    start = _drag_start(driver, tab)
    end = start[0] + 100, start[1] + 110
    driver.input(dict(type="pointerMove", x=end[0], y=end[1]))
    driver.wait("active tab drag", lambda: driver.snapshot()["DragActive"])
    driver.key("Cmd+W")
    _check(driver.snapshot()["Tabs"] == 1, "tab closed before its drag source was released")
    _release(driver, end)
    _press(driver, "关闭选项卡")
    driver.wait("closed dragged tab", lambda: driver.snapshot()["Tabs"] == 0)
    driver.key("Cmd+Shift+T")
    _authenticate(driver, lambda state: len(state.get("Sessions") or []) == 1 and
                  state["Sessions"][0]["State"] == CONNECTED)
    driver.focus_terminal()
    driver.command("printf '\\nCORE_E2E_REOPEN_OK\\n'")
    driver.output("\nCORE_E2E_REOPEN_OK\n")
    print("PASS core: SSH split isolation, surviving transport, dragged close and reopen", flush=True)


def _paused(driver, name):
    _task(driver, name, lambda task: task["State"] == CONNECTED and task["Transferred"] > 0)
    _press(driver, "全部暂停")
    task = _task(driver, name, lambda item: item["State"] == PAUSED and not item["RunPresent"])
    _check(task["TemporaryCount"] > 0, "paused transfer has no tracked staging file")
    return task


def _transfer_control(driver, task, label):
    def locate():
        titles = _nodes(driver, task["Name"], ("text",))
        controls = _nodes(driver, label, ("button",))
        pairs = [(abs(_center(control)[1] - _center(title)[1]), control)
                 for title in titles for control in controls]
        if not pairs:
            return None
        distance, control = min(pairs, key=lambda pair: pair[0])
        return control if distance < 24 else None
    control = driver.wait("task " + str(task["ID"]) + " control " + label, locate)
    x, y = _center(control)
    driver.input(dict(type="click", x=x, y=y))


def _refresh_during_drag(driver, fixture, name):
    row = _row(driver, name, True)
    pane = _file(driver)["RemoteBounds"]
    refresh = next(node for node in _nodes(driver, "刷新", ("button",))
                   if pane["x"] <= _center(node)[0] < pane["x"] + pane["width"])
    _docker(fixture, "mv", fixture["root"] + "/" + name, fixture["root"] + "/after-refresh.txt")
    subprocess.run(["docker", "pause", fixture["container"]], check=True, capture_output=True)
    try:
        x, y = _center(refresh)
        driver.input(dict(type="click", x=x, y=y))
        driver.wait("pending real FTP refresh", lambda: _file(driver)["Loading"])
        held = _drag_start(driver, row)
    finally:
        subprocess.run(["docker", "unpause", fixture["container"]], check=True, capture_output=True)
    driver.wait("directory response held behind drag", lambda:
                driver.snapshot()["FileDragPendingDirectory"])
    before = {task["ID"] for task in _tasks(driver)}
    _release(driver, held)
    driver.wait("refreshed directory after release", lambda:
                any(entry["Name"] == "after-refresh.txt" for entry in _file(driver)["Entries"])
                and not driver.snapshot()["DragActive"])
    _check({task["ID"] for task in _tasks(driver)} == before, "canceled refresh drag created a transfer")
    print("PASS core: real FTP refresh cannot remove a held drag source or drop stale data", flush=True)


def run_core(driver):
    local = driver.config_root / "local"
    expected = _prepare_data(driver, local)
    _ssh_lifecycle(driver)
    for protocol, name in (("sftp", "Core SFTP"), ("ftp", "Core FTP")):
        _create_site(driver, name, protocol, local)
        upload, download = "core-" + protocol + "-upload.txt", "core-" + protocol + "-download.txt"
        _transfer(driver, upload, False)
        _task(driver, upload, lambda task: task["State"] == DONE)
        _check(_remote_hash(_fixture(driver, protocol), upload) == expected[upload],
               protocol + " upload bytes differ")
        _transfer(driver, download, True)
        _task(driver, download, lambda task: task["State"] == DONE)
        _check(_hash(local / download) == expected[download], protocol + " download bytes differ")
        _file_ready(driver, name)
        if protocol == "sftp":
            before = {task["ID"] for task in _tasks(driver)}
            held = _drag_start(driver, _row(driver, upload, False))
            driver.key("Cmd+Alt+1")
            _check(driver.snapshot()["Section"] == 1, "file page changed before drag release")
            _release(driver, held)
            driver.wait("page switch after drag release", lambda: driver.snapshot()["Section"] == 0)
            _check({task["ID"] for task in _tasks(driver)} == before, "page-switch drag created a transfer")
            driver.key("Cmd+Alt+2")
            _file_ready(driver, name)
        print("PASS core: " + protocol + " native drag upload/download and SHA-256", flush=True)

    _press(driver, "清除已完成")
    _transfer(driver, "core-resume.bin", True)
    _paused(driver, "core-resume.bin")
    _press(driver, "全部继续")
    _task(driver, "core-resume.bin", lambda task: task["State"] == DONE, timeout=90)
    _check(_hash(local / "core-resume.bin") == expected["core-resume.bin"], "resumed download bytes differ")
    _transfer(driver, "core-cancel.bin", True)
    cancel = _paused(driver, "core-cancel.bin")
    _transfer_control(driver, cancel, "取消传输")
    driver.wait("canceled task removed", lambda: all(task["ID"] != cancel["ID"] for task in _tasks(driver)))
    driver.wait("owned parts cleaned", lambda: not list(local.glob(".burrow-*.part")))
    _check(not (local / "core-cancel.bin").exists(), "canceled download published a final file")
    print("PASS core: actual pause/resume bytes and canceled staging-file cleanup", flush=True)

    owner = _file(driver, "Core FTP")
    owned = {task["ID"] for task in _tasks(driver) if task["ViewID"] == owner["ID"]}
    _check(bool(owned), "task-isolation fixture has no tasks owned by the original tab")
    _catalog_open(driver, "Core FTP")
    _authenticate(driver, lambda state: any(item["Selected"] and item["ID"] != owner["ID"]
                  and item["State"] == CONNECTED for item in state.get("FileSessions") or []))
    other = _file_ready(driver, "Core FTP")
    _check(other["ID"] != owner["ID"] and other["Host"] == owner["Host"],
           "task-isolation fixture did not open two views of the same saved profile")
    _check(not any(task["ViewID"] == other["ID"] for task in _tasks(driver)),
           "new same-profile tab inherited another tab's transfers")
    driver.wait("empty task dock for a second view of the same profile", lambda:
                bool(_nodes(driver, "暂无传输任务", ("text",))))
    _select_file_tab(driver, owner["ID"])
    _file_ready(driver, "Core FTP")
    _check({task["ID"] for task in _tasks(driver) if task["ViewID"] == owner["ID"]} == owned,
           "switching tabs changed task ownership")
    _refresh_during_drag(driver, _fixture(driver, "ftp"), "core-ftp-download.txt")
    _transfer(driver, "core-restart.bin", True)
    paused = _paused(driver, "core-restart.bin")
    checkpoint = dict(local=str(local), expected=expected, task_id=paused["ID"],
                      owner=paused["ViewRestoreID"], transferred=paused["Transferred"])
    _check(checkpoint["owner"] != "", "restart task has no stable owning tab identity")
    print("PASS core: same-endpoint task isolation; paused checkpoint ready for normal quit", flush=True)
    driver.quit()
    return checkpoint


def run_core_recovered(driver, checkpoint):
    state = driver.snapshot()
    task = next((item for item in state.get("TransferTasks") or []
                 if item["ID"] == checkpoint["task_id"]), None)
    _check(task is not None and task["State"] == PAUSED and not task["RunPresent"],
           "normal restart did not restore the paused task without starting it")
    _check(task["ViewRestoreID"] == checkpoint["owner"] and task["Transferred"] == checkpoint["transferred"],
           "normal restart changed transfer ownership or its confirmed offset")
    owner = next((item for item in state.get("FileSessions") or []
                  if item["RestoreID"] == checkpoint["owner"]), None)
    _check(owner is not None and owner["State"] == 0 and state["Prompts"] == 0,
           "workspace restoration connected or authenticated automatically")
    _select_file_tab(driver, owner["ID"])
    _press(driver, "全部继续")
    _authenticate(driver, lambda observed: next((item["State"] == DONE or
        item["Transferred"] > checkpoint["transferred"] for item in
        observed.get("TransferTasks") or [] if item["ID"] == checkpoint["task_id"]), False))
    _task(driver, "core-restart.bin", lambda item: item["State"] == DONE, timeout=90)
    local = Path(checkpoint["local"])
    _check(_hash(local / "core-restart.bin") == checkpoint["expected"]["core-restart.bin"],
           "restarted transfer bytes differ")
    _check(not list(local.glob(".burrow-*.part")), "completed restored transfer retained staging files")
    print("PASS core: normal quit/restart, offline task binding, explicit resume and SHA-256", flush=True)
    driver.quit()


def _pane_press(driver, label, remote=True):
    pane = _file(driver)["RemoteBounds" if remote else "LocalBounds"]
    controls = [node for node in _nodes(driver, label, ("button",))
                if pane["x"] <= _center(node)[0] < pane["x"] + pane["width"]
                and pane["y"] <= _center(node)[1] < pane["y"] + pane["height"]]
    _check(len(controls) == 1, "expected one pane control: " + label)
    x, y = _center(controls[0])
    driver.input(dict(type="click", x=x, y=y))


def _browse_home(driver, name):
    initial = _file_ready(driver, name)
    expected = {"hello.txt", "你好.txt", "nested", "denied"}
    _check(expected.issubset({entry["Name"] for entry in initial["Entries"]}),
           "fixture home entries are missing")
    x, y = _center(_row(driver, "nested", True))
    driver.input(dict(type="click", x=x, y=y, clicks=2))
    nested = driver.wait("nested directory loaded", lambda:
        item if (item := _file(driver, name)) and not item["Loading"]
        and item["Path"] != initial["Path"] else None)
    _check(nested["State"] == CONNECTED and not nested["Error"]
           and any(entry["Name"] == "inside.txt" for entry in nested["Entries"]),
           "directory navigation did not load the fixture child")
    _pane_press(driver, "后退")
    driver.wait("home history restored", lambda:
                (_file(driver, name) or {}).get("Path") == initial["Path"])
    _file_ready(driver, name)
    x, y = _center(_row(driver, "denied", True))
    driver.input(dict(type="click", x=x, y=y, clicks=2))
    denied = driver.wait("directory permission refusal", lambda:
        item if (item := _file(driver, name)) and not item["Loading"] and item["Error"] else None)
    _check(denied["State"] == CONNECTED and denied["Path"] == initial["Path"],
           "permission refusal changed the last successful path or disconnected the site")
    driver.wait("visible permission error", lambda:
                any(node.get("label", "").startswith("读取目录失败") for node in driver.nodes()))
    _pane_press(driver, "刷新")
    restored = _file_ready(driver, name)
    _check(restored["Path"] == initial["Path"], "refresh did not restore the successful directory")
    _node(driver, "上传文件")
    _pane_press(driver, "新建文件夹")
    _node(driver, "新建文件夹", ("dialog",))
    _press(driver, "取消")
    driver.wait("folder dialog dismissed", lambda:
                not _nodes(driver, "新建文件夹", ("dialog",)))
    before = driver.snapshot()["Transfers"]
    was_open = driver.snapshot()["LocalOpen"]
    driver.key("Cmd+Shift+L")
    driver.wait("local panel toggled", lambda: driver.snapshot()["LocalOpen"] != was_open)
    driver.key("Cmd+Shift+L")
    driver.wait("local panel restored", lambda: driver.snapshot()["LocalOpen"] == was_open)
    _check(driver.snapshot()["Transfers"] == before, "browsing controls unexpectedly queued a transfer")
    print("PASS native files: " + name + " home, Unicode, history, permission refusal and current controls", flush=True)
    return restored


def _close_file(driver):
    session = _file(driver)
    driver.key("Cmd+W")
    driver.wait("file close request", lambda:
                not any(item["ID"] == session["ID"] for item in driver.snapshot()["FileSessions"])
                or bool(_nodes(driver, "关闭选项卡", ("button",))))
    if _nodes(driver, "关闭选项卡", ("button",)):
        _press(driver, "关闭选项卡")
    driver.wait("file tab released", lambda:
                not any(item["ID"] == session["ID"] for item in driver.snapshot()["FileSessions"]))


def run_file_browsing(driver):
    """Current saved-site UI; cross-page quick-link reuse is not covered here."""
    ftp = _fixture(driver, "ftp")
    driver.ftp_container = ftp["container"]
    local = driver.config_root / "browse-local"
    local.mkdir(mode=0o700, parents=True, exist_ok=True)
    _check(driver.server_connections()[1] == 0, "SSH fixture has pre-existing connections")
    _check(driver.established_connections(ftp["container"], 21) == 0,
           "FTP fixture has pre-existing connections")
    driver.key("Cmd+Alt+1")
    driver.wait("SSH page", lambda: driver.snapshot()["Section"] == 0)
    _catalog_open(driver, "Docker Password")
    _authenticate(driver, lambda state: len(state.get("Sessions") or []) == 1
                  and state["Sessions"][0]["State"] == CONNECTED)
    driver.focus_terminal()
    driver.command("printf '\\nBROWSE_SSH_READY\\n'")
    driver.output("\nBROWSE_SSH_READY\n")
    ssh = driver.focused()

    sftp = _create_site(driver, "Browse SFTP", "sftp", local, remote_directory="/home/burrow")
    _check(not sftp["Temporary"] and sftp["Connection"] != ssh["Connection"],
           "saved SFTP site did not open its own session")
    _check(any(profile["Name"] == "Browse SFTP" for profile in driver.snapshot()["FileProfiles"]),
           "SFTP site was not saved through the form")
    _browse_home(driver, "Browse SFTP")
    driver.wait("independent SSH and saved SFTP transports", lambda: driver.server_connections()[1] == 2)

    _create_site(driver, "Browse FTP", "ftp", local, remote_directory="/")
    _close_file(driver)
    driver.wait("first FTP connection released", lambda:
                driver.established_connections(ftp["container"], 21) == 0)
    _catalog_open(driver, "Browse FTP")
    _fill(driver, "输入 FTP 密码", (driver.task_dir / "password").read_text().strip() + "-invalid")
    _press(driver, "连接")
    failed = driver.wait("wrong FTP password rejected", lambda:
        item if (item := _file(driver, "Browse FTP")) and item["State"] == 3 else None)
    _check(bool(failed["Error"]), "FTP authentication failure has no visible explanation")
    _node(driver, "连接失败", ("text",))
    _close_file(driver)
    driver.wait("failed FTP connection released", lambda:
                driver.established_connections(ftp["container"], 21) == 0)
    _catalog_open(driver, "Browse FTP")
    _authenticate(driver, lambda state: any(item["Selected"] and item["Name"] == "Browse FTP"
                  and item["State"] == CONNECTED for item in state["FileSessions"]))
    current_ftp = _browse_home(driver, "Browse FTP")
    _check(current_ftp["Connection"] == 0 and not current_ftp["Temporary"],
           "saved FTP site incorrectly acquired an SSH transport")
    print("PASS native files: FTP wrong password rejection followed by a successful new connection", flush=True)

    _select_file_tab(driver, sftp["ID"])
    _close_file(driver)
    driver.wait("SFTP transport released without closing SSH", lambda: driver.server_connections()[1] == 1)
    _select_file_tab(driver, current_ftp["ID"])
    _pane_press(driver, "刷新")
    _file_ready(driver, "Browse FTP")
    _check(driver.established_connections(ftp["container"], 21) == 1,
           "closing SFTP disconnected the FTP session")
    driver.key("Cmd+Alt+1")
    driver.wait("surviving SSH page", lambda: driver.snapshot()["Section"] == 0)
    driver.focus_terminal()
    driver.command("printf '\\nBROWSE_SSH_SURVIVES\\n'")
    driver.output("\nBROWSE_SSH_SURVIVES\n")
    driver.key("Cmd+W")
    _press(driver, "关闭选项卡")
    driver.wait("SSH session closed", lambda: not driver.sessions())
    driver.wait("last SSH transport released", lambda: driver.server_connections()[1] == 0)
    _check(driver.established_connections(ftp["container"], 21) == 1,
           "closing SSH disconnected the independent FTP session")
    driver.key("Cmd+Alt+2")
    driver.wait("surviving FTP page", lambda: driver.snapshot()["Section"] == 1)
    _pane_press(driver, "刷新")
    _file_ready(driver, "Browse FTP")
    _close_file(driver)
    driver.wait("last FTP transport released", lambda:
                driver.established_connections(ftp["container"], 21) == 0)
    _check(driver.snapshot()["Transfers"] == 0, "file browsing unexpectedly created transfers")
    print("PASS native files: saved SSH/SFTP/FTP independent close and actual transport release", flush=True)
    print("COVERAGE native files: current saved-site UI; cross-page quick-link reuse is outside this mode", flush=True)
    driver.quit()
