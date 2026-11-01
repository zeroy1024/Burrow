#!/usr/bin/env python3
"""Start an isolated local OpenSSH fixture without storing credentials in the repo."""

import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time


def fixture_files(container):
    subprocess.run(["docker", "exec", "-i", container, "sh"], input=b"""set -eu
install -d -o sshdesk -g sshdesk -m 0755 /home/sshdesk/nested
printf 'integration hello\\n' > /home/sshdesk/hello.txt
printf 'nested entry\\n' > /home/sshdesk/nested/inside.txt
install -d -m 000 /home/sshdesk/denied
""", check=True)
    subprocess.run(["docker", "exec", "-i", container, "tee", "/home/sshdesk/你好.txt"],
                   input=b"unicode entry\n", stdout=subprocess.DEVNULL, check=True)
    subprocess.run(["docker", "exec", container, "chown", "-R", "sshdesk:sshdesk", "/home/sshdesk"], check=True)


def passive_range():
    for _ in range(20):
        first = 40000 + secrets.randbelow(10000)
        reserved = []
        try:
            for port in range(first, first + 5):
                listener = socket.socket()
                reserved.append(listener)
                listener.bind(("127.0.0.1", port))
            return first, first + 4
        except OSError:
            continue
        finally:
            for listener in reserved:
                listener.close()
    raise RuntimeError("no free local passive-port range for the FTP fixture")


def start_ftp(project):
    runtime = Path(os.environ["SSHDESK_TEST_DIR"]).resolve()
    password = (runtime / "password").read_text()
    image = "sshdesk/integration-ftpd:local"
    subprocess.run(["docker", "build", "-t", image, str(project / "dev/ftp-integration")], check=True)
    first, last = passive_range()
    name = "sshdesk-ftp-integration-" + secrets.token_hex(4)
    subprocess.run(["docker", "run", "-d", "--init", "--name", name, "--label", "dev.sshdesk.integration=true",
                    "--publish", "127.0.0.1::21", "--publish", f"127.0.0.1:{first}-{last}:{first}-{last}",
                    "--env", f"SSHDESK_PASV_START={first}", "--env", f"SSHDESK_PASV_END={last}", image],
                   check=True, stdout=subprocess.DEVNULL)
    subprocess.run(["docker", "exec", "-i", name, "chpasswd"],
                   input=("sshdesk:" + password + "\n").encode(), check=True)
    fixture_files(name)
    address = subprocess.check_output(["docker", "port", name, "21/tcp"], text=True).strip()
    state = dict(container=name, address=address, directory=str(runtime), user="sshdesk", passive=[first, last])
    (runtime / "ftp-server.json").write_text(json.dumps(state, indent=2) + "\n")
    host, port = address.rsplit(":", 1)
    with socket.create_connection((host, int(port)), timeout=5) as connection:
        if not connection.recv(128).startswith(b"220"):
            raise RuntimeError("FTP fixture did not send a greeting")
    print(json.dumps(state))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ftp", action="store_true", help="start FTP using SSHDESK_TEST_DIR credentials")
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    if args.ftp:
        start_ftp(project)
        return
    runtime = Path(tempfile.mkdtemp(prefix="sshdesk-integration-", dir="/private/tmp"))
    os.chmod(runtime, 0o700)
    for name in ("password", "key_passphrase"):
        path = runtime / name
        path.write_text(secrets.token_urlsafe(24))
        os.chmod(path, 0o600)
    for name, passphrase in (
        ("key_plain", ""),
        ("key_encrypted", (runtime / "key_passphrase").read_text()),
    ):
        generated = subprocess.run(
            ["ssh-keygen", "-q", "-t", "ed25519", "-f", str(runtime / name), "-N", passphrase],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        if generated.returncode:
            raise RuntimeError("fixture key generation failed")
    authorized = runtime / "authorized_keys"
    authorized.write_bytes((runtime / "key_plain.pub").read_bytes() + (runtime / "key_encrypted.pub").read_bytes())
    image = "sshdesk/integration-sshd:local"
    subprocess.run(["docker", "build", "-t", image, str(project / "dev/ssh-integration")], check=True)
    name = "sshdesk-integration-" + secrets.token_hex(4)
    subprocess.run(
        ["docker", "run", "-d", "--name", name, "--label", "dev.sshdesk.integration=true",
         "--publish", "127.0.0.1::22", image],
        check=True, stdout=subprocess.DEVNULL,
    )
    # Only disposable public keys cross into the local Docker fixture.
    subprocess.run(["docker", "cp", str(authorized), name + ":/tmp/authorized_keys"], check=True)
    subprocess.run(
        ["docker", "exec", name, "install", "-o", "sshdesk", "-g", "sshdesk", "-m", "0600",
         "/tmp/authorized_keys", "/home/sshdesk/.ssh/authorized_keys"], check=True,
    )
    subprocess.run(
        ["docker", "exec", "-i", name, "chpasswd"],
        input=("sshdesk:" + (runtime / "password").read_text() + "\n").encode(), check=True,
    )
    fixture_files(name)
    address = subprocess.check_output(["docker", "port", name, "22/tcp"], text=True).strip()
    state = {"container": name, "address": address, "directory": str(runtime), "user": "sshdesk"}
    (runtime / "server.json").write_text(json.dumps(state, indent=2) + "\n")
    host, port = address.rsplit(":", 1)
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, int(port)), timeout=1) as connection:
                if connection.recv(128).startswith(b"SSH-"):
                    print(json.dumps(state))
                    return
        except OSError:
            time.sleep(0.1)
    raise RuntimeError("OpenSSH fixture did not become ready; inspect the task container logs")


if __name__ == "__main__":
    main()
