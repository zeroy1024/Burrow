"""Exercise migration through the native Settings UI with isolated device data."""

from contextlib import closing
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import sqlite3
import tempfile

from ssh_e2e_coverage import _add_ssh_host, _checked, _preferences, _section, _settings_page
from ssh_e2e_scenarios import _catalog_open, _center, _check, _fill, _node, _press


SOURCE_HOST = "Migration source host"
TARGET_HOST = "Migration target host"
EXPORT = "导出迁移包…"
IMPORT = "导入迁移包…"
PACKAGE_FIELD = "迁移包密码"
CONFIRM = "替换当前设备的数据？"
UPDATE_FILE = "Burrow/updater.json"


def _select_path(driver, path):
    (driver.config_root / "migration-selection").write_text(str(path))


def _theme(driver, label, value):
    _settings_page(driver, "外观")
    _press(driver, label, ("radio",))
    driver.wait("saved appearance preference", lambda: _preferences(driver).get("appTheme") == value)


def _migration_settings(driver):
    _settings_page(driver, "通用")
    _fill(driver, "搜索设置", "迁移")


def _banner(driver, text):
    driver.wait("migration message " + text, lambda: any(
        text in node.get("label", "") for node in driver.nodes() if node.get("role") in ("status", "text")
    ), timeout=45)


def _close_dialog(driver, title):
    _press(driver, "取消")
    driver.wait("migration dialog closed", lambda: not driver.node(title, "dialog"))


def _connect(driver, name, trust=False):
    _section(driver, "SSH")
    _catalog_open(driver, name)

    def connected():
        _check(not driver.node("输入 SSH 密码", "textField"), "saved migration credential was not used")
        if driver.node("信任并连接", "button"):
            _check(trust, "saved host trust was not migrated")
            _press(driver, "信任并连接")
        return any(session["Name"] == name and session["State"] == 1 for session in driver.sessions())

    driver.wait("saved credential authenticated to the SSH fixture", connected, timeout=30)
    driver.shell_ready()
    driver.focus_terminal(name)
    driver.command("printf '\\n%s\\n' BURROW_MIGRATION_OK")
    driver.output("\nBURROW_MIGRATION_OK\n")
    driver.close_tab(name)
    driver.wait("test SSH session closed", lambda: not driver.sessions())


def _has_profile(driver, name):
    return any(profile["Name"] == name for profile in driver.profiles(0))


def _export(driver, bundle, password, host, port):
    _add_ssh_host(driver, SOURCE_HOST, host, port, password=(driver.task_dir / "password").read_text().strip())
    # Write a real host trust record and confirm the saved credential is usable.
    _connect(driver, SOURCE_HOST, trust=True)
    _theme(driver, "深色", "dark")
    _migration_settings(driver)
    _select_path(driver, bundle)
    _press(driver, EXPORT)
    dialog = _node(driver, EXPORT, ("dialog",))
    _fill(driver, PACKAGE_FIELD, password)
    _fill(driver, "确认密码", password)
    nodes = {node["id"]: node for node in driver.nodes()}
    for node in nodes.values():
        if node.get("role") != "button" or node.get("label") != EXPORT:
            continue
        parent = nodes.get(node.get("parent"))
        while parent is not None and parent["id"] != dialog["id"]:
            parent = nodes.get(parent.get("parent"))
        if parent is not None:
            x, y = _center(node)
            driver.input(dict(type="click", x=x, y=y))
            break
    else:
        raise RuntimeError("export confirmation button missing from the migration dialog")
    driver.wait("encrypted migration package written", lambda:
                bundle.is_file() and bundle.stat().st_size > 0 and not driver.node(EXPORT, "dialog"), timeout=45)
    raw = bundle.read_bytes()
    _check(SOURCE_HOST.encode() not in raw, "exported package contains the source catalog in plaintext")
    driver.menu("Quit")


def _open_import(driver, path):
    _select_path(driver, path)
    _press(driver, IMPORT)
    _node(driver, IMPORT, ("dialog",))


def _validate_import(driver, package, password):
    _open_import(driver, package)
    _fill(driver, PACKAGE_FIELD, password)
    _press(driver, "继续")
    driver.wait("migration overwrite confirmation", lambda: driver.node(CONFIRM, "dialog"), timeout=45)


def _import(driver, bundle, password, host, port):
    _add_ssh_host(driver, TARGET_HOST, host, port, group="Migration receiver")
    _theme(driver, "浅色", "light")
    _migration_settings(driver)
    header, newline, ciphertext = bundle.read_bytes().partition(b"\n")
    prefix, version = header.rsplit(b" ", 1)
    versions = (
        (b"0.0.0-0", "不支持导入由 0.0.0-0 导出的迁移包"),
        (b"999999.0.0", "请升级到导出时使用的 Burrow 999999.0.0"),
    )
    _check(newline and prefix.startswith(b"BURROW-MIGRATION-") and all(version != item[0] for item in versions),
           "unexpected fixture versions")
    for number, (other_version, message) in enumerate(versions):
        other = bundle.with_name("version-" + str(number) + ".burrow-migration")
        other.write_bytes(prefix + b" " + other_version + newline + ciphertext)
        _open_import(driver, other)
        _banner(driver, message)
        _check(not driver.node(PACKAGE_FIELD, "textField"), "wrong-version package reached decryption")
        _close_dialog(driver, IMPORT)

    _open_import(driver, bundle)
    _fill(driver, PACKAGE_FIELD, password + "-wrong")
    _press(driver, "继续")
    _banner(driver, "无法解密迁移包")
    _close_dialog(driver, IMPORT)

    _validate_import(driver, bundle, password)
    _close_dialog(driver, CONFIRM)
    _check(_has_profile(driver, TARGET_HOST) and not _has_profile(driver, SOURCE_HOST)
           and _preferences(driver)["appTheme"] == "light", "failed or cancelled import modified the target")
    print("PASS migration UI: both version warnings, wrong password, and cancellation keep target data", flush=True)

    _validate_import(driver, bundle, password)
    pid = int((driver.config_root / "app.pid").read_text())
    _press(driver, "替换并重启")
    return pid


def _after_relaunch(driver):
    _check(_has_profile(driver, SOURCE_HOST) and not _has_profile(driver, TARGET_HOST),
           "restarted device did not switch to the imported catalog")
    _check(_preferences(driver)["appTheme"] == "dark", "imported preferences were not loaded")
    _check(json.loads((driver.config_root / UPDATE_FILE).read_text()).get("skippedVersion") == "0.0.1",
           "update settings outside SQLite were not migrated")
    _settings_page(driver, "外观")
    _check(_checked(driver, "深色", "radio"), "native UI did not apply the imported theme")
    _connect(driver, SOURCE_HOST)
    backups = list((driver.config_root / "Burrow").glob("migration-backup-*/previous"))
    _check(len(backups) == 1, "the previous target data was not backed up")
    # Open a copy as a normal SQLite database so WAL recovery can create its
    # sidecars and the saved backup itself stays untouched.
    with tempfile.TemporaryDirectory(dir=driver.config_root, prefix="backup-check-") as directory:
        copy = Path(directory)
        for suffix in ("", "-wal", "-shm", "-journal"):
            name = "burrow.db" + suffix
            source = backups[0] / name
            if not suffix or source.exists():
                shutil.copyfile(source, copy / name)
        with closing(sqlite3.connect((copy / "burrow.db").as_uri() + "?mode=rw", uri=True)) as db:
            names = {json.loads(row[0])["name"] for row in db.execute("SELECT value FROM profiles")}
            _check(TARGET_HOST in names and SOURCE_HOST not in names, "backup does not contain the previous target catalog")
            preferences = db.execute("SELECT value FROM settings WHERE key='preferences'").fetchone()
            _check(preferences and json.loads(preferences[0]).get("appTheme") == "light",
                   "backup does not contain the previous target appearance")
    print("PASS migration UI: app restarted, applied settings, and authenticated with migrated credentials and trust", flush=True)
    driver.menu("Quit")


def _update_fixture(root, version):
    data = root / UPDATE_FILE
    data.parent.mkdir(mode=0o700, exist_ok=True)
    data.write_text(json.dumps(dict(automaticChecks=False, skippedVersion=version)))


def run_migration(window, integration_process, repo, bundle, env, source, run_dir, report):
    package = run_dir / "device.burrow-migration"
    target = Path(tempfile.mkdtemp(prefix="migration-target-", dir=run_dir))
    report["receiving_configuration"] = str(target)
    password = secrets.token_urlsafe(24)
    host, port = env["BURROW_TEST_ADDRESS"].rsplit(":", 1)
    _update_fixture(source, "0.0.1")
    _update_fixture(target, "0.0.2")
    target_env = dict(env, BURROW_INTEGRATION_CONFIG=str(target),
                      BURROW_INTEGRATION_RELAUNCH_LOG=str(run_dir / "native-app-migration-restored.log"))

    def record_exit(driver, phase):
        driver.wait("SSH transports released after application exit", lambda: driver.server_connections()[1] == 0, timeout=5)
        report["phases"].append(dict(phase=phase, status="passed", clean_exit=True, race_free=True, transports_released=True))

    with window(repo, bundle, env, source, run_dir, "migration-source") as driver:
        _export(driver, package, password, host, port)
    record_exit(driver, "source")
    try:
        with window(repo, bundle, target_env, target, run_dir, "migration-receiver",
                    ready_label="SSH", ready_role="tab") as driver:
            pid = _import(driver, package, password, host, port)
        record_exit(driver, "receiver")
        with window(repo, bundle, target_env, target, run_dir, "migration-restored",
                    ready_label=SOURCE_HOST, relaunched_from=pid) as driver:
            _after_relaunch(driver)
        record_exit(driver, "restored")
    finally:
        pid = integration_process(target / "app.pid", bundle)
        if pid is not None:
            try:
                os.kill(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
