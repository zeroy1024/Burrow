#!/usr/bin/env python3
"""Verify cross-build Keychain access and report Touch ID keychain capabilities."""

import json
import plistlib
import re
import secrets
import shlex
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
IDENTIFIER = "dev.burrow.app.signing-probe"
SWIFT = r"""
import Foundation
import Security
import LocalAuthentication

func output(_ value: [String: Any]) {
    let data = try! JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
    print(String(data: data, encoding: .utf8)!)
}

let action = CommandLine.arguments[1]
if action == "capabilities" {
    let context = LAContext()
    var policyError: NSError?
    let available = context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics,
                                              error: &policyError)
    var accessError: Unmanaged<CFError>?
    let access = SecAccessControlCreateWithFlags(nil,
        kSecAttrAccessibleWhenUnlockedThisDeviceOnly, .biometryCurrentSet, &accessError)
    var report: [String: Any] = [
        "biometricAvailable": available,
        "biometryType": context.biometryType.rawValue,
        "biometricError": policyError?.code ?? 0
    ]
    if let access = access {
        let account = UUID().uuidString
        let item: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "dev.burrow.app.touchid-probe",
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
            kSecAttrAccessControl as String: access,
            kSecValueData as String: Data(repeating: 0x42, count: 32),
            kSecUseAuthenticationUI as String: kSecUseAuthenticationUIFail
        ]
        let status = SecItemAdd(item as CFDictionary, nil)
        report["protectedKeychainAddStatus"] = status
        if status == errSecSuccess {
            let query: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: "dev.burrow.app.touchid-probe",
                kSecAttrAccount as String: account,
                kSecUseDataProtectionKeychain as String: true,
                kSecUseAuthenticationUI as String: kSecUseAuthenticationUIFail
            ]
            report["protectedKeychainCleanupStatus"] = SecItemDelete(query as CFDictionary)
        }
    } else {
        report["accessControlCreationFailed"] = true
    }
    output(report)
    exit(0)
}

var keychain: SecKeychain?
let opened = SecKeychainOpen(CommandLine.arguments[2], &keychain)
guard opened == errSecSuccess, let keychain = keychain else {
    output(["openStatus": opened])
    exit(2)
}
let fixture = Data("burrow-keychain-continuity-fixture".utf8)
let common: [String: Any] = [
    kSecClass as String: kSecClassGenericPassword,
    kSecAttrService as String: "dev.burrow.app.signing-probe",
    kSecAttrAccount as String: "cross-build-fixture"
]
if action == "write" {
    var item = common
    item[kSecUseKeychain as String] = keychain
    item[kSecValueData as String] = fixture
    let status = SecItemAdd(item as CFDictionary, nil)
    output(["writeStatus": status])
    exit(status == errSecSuccess ? 0 : 2)
}
var query = common
query[kSecMatchSearchList as String] = [keychain]
query[kSecReturnData as String] = true
query[kSecMatchLimit as String] = kSecMatchLimitOne
query[kSecUseAuthenticationUI as String] = kSecUseAuthenticationUIFail
var value: CFTypeRef?
let status = SecItemCopyMatching(query as CFDictionary, &value)
let matched = (value as? Data) == fixture
#if SECOND_BUILD
let build = 2
#else
let build = 1
#endif
output(["readStatus": status, "matched": matched, "build": build])
exit(status == errSecSuccess && matched ? 0 : 2)
"""


def run(arguments):
    result = subprocess.run(arguments, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        # Do not render argv: temporary keychain credentials are passed to security.
        raise RuntimeError(result.stderr.decode(errors="replace").strip()
                           or result.stdout.decode(errors="replace").strip()
                           or (Path(arguments[0]).name + " failed"))
    return result.stdout


def search_list():
    return shlex.split(run(["security", "list-keychains", "-d", "user"]).decode())


def signature(bundle):
    result = subprocess.run(["codesign", "-dvvv", "-r-", str(bundle)],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
    text = (result.stdout + result.stderr).decode()
    requirement = next(line.removeprefix("# ").strip() for line in text.splitlines()
                       if "designated =>" in line)
    match = re.search(r"^CDHash=(.+)$", text, re.M)
    if match is None:
        raise RuntimeError("codesign did not return the fixture code hash")
    digest = match.group(1)
    return requirement, digest


def main():
    if sys.platform != "darwin":
        raise RuntimeError("This verification requires macOS")
    config = json.loads((ROOT / "mygo.json").read_text())
    identity = config["macos"].get("signingIdentity")
    if not identity or identity == "-":
        raise RuntimeError("Set up the fixed signing identity first")
    root = Path(tempfile.mkdtemp(prefix="burrow-keychain-verify-"))
    print("Verification fixture:", root, flush=True)
    source = root / "probe.swift"
    source.write_text(SWIFT)
    signing_keychain = json.loads(run(["security", "default-keychain", "-d", "user"]).decode().strip())
    bundles = []
    signatures = []
    for version in (1, 2):
        bundle = root / ("version-" + str(version)) / "BurrowSigningProbe.app"
        macos = bundle / "Contents/MacOS"
        macos.mkdir(parents=True)
        info = {
            "CFBundleIdentifier": IDENTIFIER,
            "CFBundleExecutable": "Probe",
            "CFBundleName": "Burrow Signing Probe",
            "CFBundlePackageType": "APPL",
            "CFBundleVersion": str(version),
            "LSMinimumSystemVersion": "13.0",
        }
        (bundle / "Contents/Info.plist").write_bytes(plistlib.dumps(info))
        command = ["swiftc", "-suppress-warnings", "-module-cache-path", str(root / "module-cache")]
        if version == 2:
            command += ["-D", "SECOND_BUILD"]
        run([*command, str(source), "-o", str(macos / "Probe")])
        signing = ["codesign", "--force", "--options", "runtime", "--timestamp=none",
                   "--keychain", signing_keychain, "--sign", identity]
        if config["macos"].get("entitlements"):
            signing += ["--entitlements", str(ROOT / config["macos"]["entitlements"])]
        run([*signing, str(bundle)])
        run(["codesign", "--verify", "--strict", str(bundle)])
        signatures.append(signature(bundle))
        bundles.append(bundle)
    if signatures[0][0] != signatures[1][0] or signatures[0][1] == signatures[1][1]:
        raise RuntimeError("The fixtures must have different code hashes and the same designated requirement")

    keychain = root / "fixture.keychain-db"
    password = secrets.token_urlsafe(32)
    try:
        run(["security", "create-keychain", "-p", password, str(keychain)])
        installed = root / "installed/BurrowSigningProbe.app"
        installed.parent.mkdir()
        shutil.copytree(bundles[0], installed)
        executable = installed / "Contents/MacOS/Probe"
        written = json.loads(run([str(executable), "write", str(keychain)]))
        # Replace the entire bundle, retaining v1, rather than overwriting a
        # signed executable whose signature might still be cached by the kernel.
        installed.rename(root / "retained-version-1.app")
        shutil.copytree(bundles[1], installed)
        read = json.loads(run([str(executable), "read", str(keychain)]))
        capabilities = json.loads(run([str(executable), "capabilities"]))
        report = {
            "sameDesignatedRequirement": True,
            "differentCodeHashes": True,
            "keychain": {**written, **read},
            "touchID": capabilities,
            "fixtureDirectory": str(root),
        }
        (root / "result.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report, indent=2))
    finally:
        # Remove only this fixture from the search list. Preserve any unrelated
        # changes another app made while the verification was running.
        current = search_list()
        retained = [path for path in current if Path(path) != keychain]
        if retained != current:
            run(["security", "list-keychains", "-d", "user", "-s", *retained])


if __name__ == "__main__":
    try:
        main()
    except RuntimeError as error:
        raise SystemExit(str(error)) from None
