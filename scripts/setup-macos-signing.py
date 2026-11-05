#!/usr/bin/env python3
"""Create once, or verify, Burrow's persistent local code-signing identity."""

import argparse
import hashlib
import json
import os
import re
import secrets
import ssl
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
CERTIFICATE = ROOT / "assets/signing/burrow-local.pem"
IDENTITY_NAME = "Burrow Local Code Signing"


def run(arguments, *, data=None):
    return subprocess.run(arguments, input=data, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, check=True).stdout


def identities(keychain):
    output = run(["security", "find-identity", "-p", "codesigning", keychain]).decode()
    return sorted({(digest.upper(), name) for digest, name in
                   re.findall(r'([0-9A-Fa-f]{40}) "([^"]+)"', output)})


def fingerprint(certificate):
    # codesign uses SHA-1 certificate identifiers, not a password hash.
    return hashlib.sha1(ssl.PEM_cert_to_DER_cert(certificate.decode())).hexdigest().upper()


def create_certificate():
    # The unencrypted private key exists only in process memory and pipes.
    # The login Keychain owns the persistent private key.
    private_key = run(["/usr/bin/openssl", "genrsa", "3072"])
    password = secrets.token_urlsafe(32)
    configuration = """[req]
distinguished_name=subject
x509_extensions=signing
prompt=no
[subject]
CN=Burrow Local Code Signing
[signing]
basicConstraints=critical,CA:TRUE
keyUsage=critical,digitalSignature,keyCertSign
extendedKeyUsage=codeSigning
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always
"""
    with tempfile.TemporaryDirectory(prefix="burrow-signing-public-") as directory:
        config = Path(directory) / "certificate.cnf"
        config.write_text(configuration)
        certificate = run([
            "/usr/bin/openssl", "req", "-new", "-x509", "-sha256",
            "-key", "/dev/stdin", "-days", "3650", "-config", str(config),
        ], data=private_key)
        public = Path(directory) / "certificate.pem"
        public.write_bytes(certificate)
        archive = run([
            "/usr/bin/openssl", "pkcs12", "-export", "-inkey", "/dev/stdin",
            "-in", str(public), "-name", IDENTITY_NAME, "-passout", "pass:" + password,
        ], data=private_key)
    return certificate, archive, password


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true",
                        help="Verify the pinned identity without creating or changing it")
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("Code-signing identity setup requires macOS")
    keychain = json.loads(run(["security", "default-keychain", "-d", "user"]).decode().strip())
    available = identities(keychain)
    config_file = ROOT / "mygo.json"
    config = json.loads(config_file.read_text())
    if CERTIFICATE.exists():
        certificate = CERTIFICATE.read_bytes()
        expected = fingerprint(certificate)
        if expected not in {digest for digest, _ in available}:
            raise RuntimeError(
                "The pinned signing private key is not in the login Keychain. "
                "Import its existing certificate and private key; do not generate a replacement."
            )
    else:
        if args.check:
            raise RuntimeError("Run scripts/setup-macos-signing.py once to create the signing identity")
        matching = [digest for digest, name in available if name == IDENTITY_NAME]
        if len(matching) > 1:
            raise RuntimeError("More than one Burrow signing identity exists; resolve it before setup")
        if matching:
            exported = run(["security", "find-certificate", "-c", IDENTITY_NAME, "-p", keychain])
            certificates = re.findall(
                rb"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----", exported, re.S)
            certificate = next(value for value in certificates if fingerprint(value) == matching[0])
        else:
            certificate, archive, password = create_certificate()
            # The Security import command requires a regular path. The short-lived
            # archive is encrypted with a random password held only in memory.
            with tempfile.TemporaryDirectory(prefix="burrow-signing-import-") as directory:
                archive_path = Path(directory) / "identity.p12"
                descriptor = os.open(archive_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(descriptor, "wb") as source:
                    source.write(archive)
                run(["security", "import", str(archive_path), "-k", keychain,
                     "-f", "pkcs12", "-P", password, "-T", "/usr/bin/codesign"])
        expected = fingerprint(certificate)
        CERTIFICATE.parent.mkdir(parents=True, exist_ok=True)
        CERTIFICATE.write_bytes(certificate)
    if args.check:
        if config["macos"].get("signingIdentity", "").upper() != expected:
            raise RuntimeError("mygo.json does not pin the stored Burrow certificate")
    else:
        config["macos"]["signingIdentity"] = expected
        config_file.write_text(json.dumps(config, indent=2) + "\n")
    print("Burrow signing identity:", expected)
    print("Private key: default user Keychain; public certificate:", CERTIFICATE.relative_to(ROOT))


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        # Never render argv/input: an import can contain private key material.
        raise SystemExit(error.stderr.decode(errors="replace").strip()) from None
    except (RuntimeError, StopIteration) as error:
        raise SystemExit(str(error) or "The signing certificate does not match the private identity") from None
