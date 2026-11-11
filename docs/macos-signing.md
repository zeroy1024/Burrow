# macOS signing and unlock

[English](macos-signing.md) | [简体中文](macos-signing.zh.md)

Burrow macOS release packages use a fixed self-signed identity. Business passwords, managed private keys, and passphrases remain encrypted in SQLite. Automatic unlock uses a private local key file on every platform; Keychain is only a one-time migration source for older vaults. App password unlock remains independent. The build machine’s signing private key is separate from runtime unlock material.

The current identity is created and pinned, with its private key in the local user's default Keychain. Isolated validation on 2026-11-05 passed: two signed builds with different code hashes retained the same designated requirement; the second read the first's fixture without allowing another authorization dialog. Touch ID hardware was available, but creating a protected item with the self-signed probe returned -34018. Fingerprint unlock is not integrated.

## Signing identity

- [mygo.json](../mygo.json) pins the certificate fingerprint to avoid selecting a different certificate with the same name.
- [assets/signing/burrow-local.pem](../assets/signing/burrow-local.pem) contains only the public certificate and may be committed.
- Private keys never enter the project, package, commit, configuration, or logs. Setup generates the unencrypted key only in memory; temporary PKCS#12 files use a random password held only in the process.
- Self-signing is not Developer ID, provides no Apple notarization, and does not remove Gatekeeper checks.
- A fixed identity preserves access identity when reading legacy login Keychain items; it does not grant restricted data-protection Keychain entitlements. Normal file-based unlock does not depend on it.

Initial setup:

~~~sh
python3 scripts/setup-macos-signing.py
python3 scripts/setup-macos-signing.py --check
~~~

When the repository contains a public certificate, the script accepts only its existing matching private key. A build machine missing it must import the original identity, not delete/regenerate the certificate to bypass checks. Initial creation imports into the default user Keychain and allows only system codesign to access the new key.

First use may show a system codesign authorization dialog. Enter the Keychain password personally in that dialog and choose Always Allow. Do not send the password through chat, command lines, or scripts. This authorizes the new signing key, not unrestricted access to business credentials.

Use Keychain Access to export and securely back up the certificate/private key as encrypted PKCS#12. CI and other release machines must import the same identity. GitHub uses hosted `macos-15`, importing Base64 PKCS#12 and its export password from two repository Secrets into a temporary Keychain; see [GitHub CI](development.md#github-ci). Ordinary users install signed packages without importing release private keys.

## Release and development builds

[legacyUnlockKeys](../internal/desktop/vault_keys_darwin.go) supplies a read-only migration provider for `dev.burrow.app.vault-unlock` in packaged release apps named Burrow. The provider reads Keychain only when an old vault header explicitly names it. Existing file-backed and password vaults never read it. All new automatic unlock keys use private files, including release, development, and integration builds; their data namespaces remain separate.

Normal release commands and direct MyGo builds use the fixed identity. Full macOS packaging checks it first and fails explicitly if missing. Temporary isolated integration packages retain their existing temporary signing and test namespaces.

Vault metadata v2 authenticates the file/keychain source. An old Keychain-backed vault first unlocks with its original key, then rotates data keys and records into a file-backed generation. New files use Unix `0600` permissions (`0700` directories) or private Windows ACLs, are synced and read back before database references change, and the committed generation is verified. Existing v1/v2 file-backed vaults stay unchanged. Missing or unreadable keys block startup rather than trying another source or generating replacements. Migration may still need one final system authorization; later startup and upgrades do not access Keychain.

Enabling/changing an app password still re-encrypts credentials and rotates the data key; disabling it creates a private file key. Inactive local files can be recycled after commit/readback through the existing settings flow. Old Keychain items are retained for old backups and are not deleted by migration.

## Terminal libraries and library validation

Fixed-identity builds and complete-package signature checks succeed. An isolated loader found that hardened runtime rejects libghostty-vt because the self-signed certificate has no Apple Team ID, reporting mismatched process/library Team IDs. Signing success does not establish terminal runtime success.

[App entitlements](../assets/macos.entitlements.plist) enable `com.apple.security.cs.disable-library-validation` while retaining other hardened-runtime restrictions. This permits the entire Burrow process, not just Ghostty, to load libraries without matching Team IDs. The project owner explicitly authorized this exception, which `mygo.json` supplies to signing.

The final entitlements passed complete-package signature checks, actual terminal-library loading, and creation/readback/reopen/cleanup of real MyGo Keychain wrapping keys after active references ended, using temporary data and isolated services. The real Burrow workspace was not started and business credentials were not migrated.

The exception solves terminal-library loading for the current self-signed package; it does not grant Touch ID data-protection Keychain access. Retaining library validation requires signing with a valid Apple Team ID, not fabricating permission through self-signed certificate fields.

## Portable backups

Automatic-mode `Vault.Backup` uses the current in-memory data key to create an independent file unlock key for a consistent database snapshot and rewrap its metadata. Record versions/generations remain unchanged. Another machine need not access the original Mac's Keychain.

After file migration, quit normally and copy the entire data directory, including `vault-keys/`, or use a paired Vault backup. Password-mode backups require their corresponding password. An unmigrated Keychain-backed database still needs its original key or a portable backup. There is no new backup UI at present.

## Cross-version validation

This diagnostic covers the legacy Keychain signing path; it is not required for normal file-backed unlock. The earlier isolated result below does not establish that every existing user Keychain ACL will accept an update without authorization.

~~~sh
python3 scripts/verify-macos-keychain.py
~~~

The script creates signed probes and a dedicated test Keychain only in temporary directories. It does not start real Burrow or read business credentials:

1. Compile two bundles with different code hashes and the same app identifier.
2. Sign with the fixed identity and verify identical designated requirements.
3. Write a known fixture from the first bundle to the dedicated test Keychain.
4. Replace the whole bundle with the second and read with authorization UI prohibited. It must succeed without reauthorization.
5. Remove only this test Keychain from the system search list, preserving other items and concurrent changes.

Results remain in the temporary directory. Business secrets and the test Keychain password are not logged.

## Current validation boundaries

The fixed-signature probe on 2026-11-05 reported Touch ID availability with `biometryType` set to Touch ID. Creating a biometric-protected item returned -34018, a missing entitlement. Cross-version ordinary Keychain write/read OSStatus values were both 0 and fixture contents matched. No fingerprint verification ran; this does not establish implemented Touch ID unlock.

Restricted execution sandboxes may block LocalAuthentication or Keychain XPC and produce 4099 or -25291. Such results do not establish missing hardware. Run validation in the logged-in user's native context.

## Conditions for Touch ID integration

Fixed self-signing and authorization for a Touch ID-protected key are separate. [Apple TN3137](https://developer.apple.com/documentation/technotes/tn3137-on-mac-keychains) describes the data-protection Keychain required for biometric-protected items. Access-group entitlements require authorization from a valid provisioning profile; self-signed certificates alone do not provide it.

The probe reads LocalAuthentication availability without requesting fingerprint verification. It also tries to create an isolated `biometryCurrentSet` fixture with interaction prohibited. Rejection records the actual OSStatus and retains app-password behavior. Successfully created fixtures clean up only their own random account, not other items.

Add Touch ID settings and unlock UI only after both the protected-key path and cross-version access are verified. Do not store an easily decryptable shortcut key in an ordinary file and substitute a fingerprint dialog for system key protection.
