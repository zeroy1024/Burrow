#!/usr/bin/env python3
"""Start an isolated local OpenSSH fixture without storing credentials in the repo."""

import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import tempfile
import time


def fixture_files(container):
    subprocess.run(["docker", "exec", "-i", container, "sh"], input=b"""set -eu
install -d -o burrow -g burrow -m 0755 /home/burrow/nested
printf 'integration hello\\n' > /home/burrow/hello.txt
chmod 0644 /home/burrow/hello.txt
printf 'nested entry\\n' > /home/burrow/nested/inside.txt
install -d -m 000 /home/burrow/denied
""", check=True)
    subprocess.run(["docker", "exec", "-i", container, "tee", "/home/burrow/你好.txt"],
                   input=b"unicode entry\n", stdout=subprocess.DEVNULL, check=True)
    subprocess.run(["docker", "exec", container, "chown", "-R", "burrow:burrow", "/home/burrow"], check=True)


def passive_range():
    # MCP tool calls and background jobs have independent FTP connections.
    # Keep room for their listeners and sockets from interrupted transfers.
    count = 64
    for _ in range(20):
        first = 40000 + secrets.randbelow(10000)
        reserved = []
        try:
            for port in range(first, first + count):
                listener = socket.socket()
                reserved.append(listener)
                listener.bind(("127.0.0.1", port))
            return first, first + count - 1
        except OSError:
            continue
        finally:
            for listener in reserved:
                listener.close()
    raise RuntimeError("no free local passive-port range for the FTP fixture")


def ftps_certificate(runtime, mode):
    directory = Path(tempfile.mkdtemp(prefix="ftps-" + mode + "-pki-", dir=runtime))
    os.chmod(directory, 0o700)
    extensions = directory / "server.ext"
    extensions.write_text("subjectAltName=IP:127.0.0.1,DNS:localhost\nextendedKeyUsage=serverAuth\n")
    commands = [
        ["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2",
         "-subj", "/CN=Burrow disposable FTPS CA", "-keyout", str(directory / "ca.key"), "-out", str(directory / "ca.pem")],
        ["openssl", "req", "-newkey", "rsa:2048", "-nodes", "-subj", "/CN=localhost",
         "-keyout", str(directory / "server.key"), "-out", str(directory / "server.csr")],
        ["openssl", "x509", "-req", "-days", "2", "-in", str(directory / "server.csr"),
         "-CA", str(directory / "ca.pem"), "-CAkey", str(directory / "ca.key"), "-CAcreateserial",
         "-extfile", str(extensions), "-out", str(directory / "server.pem")],
    ]
    for command in commands:
        subprocess.run(command, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for source in directory.iterdir():
        os.chmod(source, 0o600)
    return directory


def start_ftp(project, tls_mode="none"):
    runtime = Path(os.environ["BURROW_TEST_DIR"]).resolve()
    password = (runtime / "password").read_text()
    image = "burrow/integration-ftpd:local"
    subprocess.run(["docker", "build", "-t", image, str(project / "dev/ftp-integration")], check=True)
    first, last = passive_range()
    name = "burrow-" + ("ftp" if tls_mode == "none" else "ftps-" + tls_mode) + "-integration-" + secrets.token_hex(4)
    port = 990 if tls_mode == "implicit" else 21
    certificate = ftps_certificate(runtime, tls_mode) if tls_mode != "none" else None
    subprocess.run(["docker", "create", "--init", "--name", name, "--label", "dev.burrow.integration=true",
                    "--publish", f"127.0.0.1::{port}", "--publish", f"127.0.0.1:{first}-{last}:{first}-{last}",
                    "--env", f"BURROW_PASV_START={first}", "--env", f"BURROW_PASV_END={last}",
                    "--env", f"BURROW_FTP_TLS={tls_mode}", image],
                   check=True, stdout=subprocess.DEVNULL)
    if certificate is not None:
        for source, destination in (("server.pem", "/run/ftps-server.pem"), ("server.key", "/run/ftps-server.key")):
            subprocess.run(["docker", "cp", str(certificate / source), name + ":" + destination], check=True)
    subprocess.run(["docker", "start", name], check=True, stdout=subprocess.DEVNULL)
    subprocess.run(["docker", "exec", "-i", name, "chpasswd"],
                   input=("burrow:" + password + "\n").encode(), check=True)
    fixture_files(name)
    address = subprocess.check_output(["docker", "port", name, str(port) + "/tcp"], text=True).strip()
    state = dict(container=name, address=address, directory=str(runtime), user="burrow", passive=[first, last])
    metadata = "ftp-server.json"
    if certificate is not None:
        state.update(tls_mode=tls_mode, ca=str(certificate / "ca.pem"), certificate=str(certificate / "server.pem"))
        metadata = "ftps-implicit-server.json" if tls_mode == "implicit" else "ftps-server.json"
    (runtime / metadata).write_text(json.dumps(state, indent=2) + "\n")
    host, port = address.rsplit(":", 1)
    with socket.create_connection((host, int(port)), timeout=5) as connection:
        if tls_mode == "implicit":
            with ssl.create_default_context(cafile=state["ca"]).wrap_socket(connection, server_hostname=host) as secure:
                greeting = secure.recv(128)
        else:
            greeting = connection.recv(128)
        if not greeting.startswith(b"220"):
            raise RuntimeError("FTP fixture did not send a greeting")
    print(json.dumps(state))


def container_address(container):
    return subprocess.check_output(
        ["docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container],
        text=True).strip()


def start_gateways(project):
    """Start an SSH jump host, a SOCKS5 proxy and a WireGuard peer beside the SSH fixture.

    The route target is the SSH fixture's bridge address, which the host cannot
    reach directly; a successful connection therefore proves the gateway path.
    """
    runtime = Path(os.environ["BURROW_TEST_DIR"]).resolve()
    target = json.loads((runtime / "server.json").read_text())
    password = (runtime / "password").read_text()
    suffix = secrets.token_hex(4)
    containers = []
    # The second label lets a caller remove the gateways of one SSH fixture.
    labels = ["--label", "dev.burrow.integration=true", "--label", "dev.burrow.gateway=" + target["container"]]

    jump = "burrow-jump-integration-" + suffix
    subprocess.run(["docker", "run", "-d", "--name", jump, *labels,
                    "--publish", "127.0.0.1::22", "burrow/integration-sshd:local"],
                   check=True, stdout=subprocess.DEVNULL)
    containers.append(jump)
    subprocess.run(["docker", "cp", str(runtime / "authorized_keys"), jump + ":/tmp/authorized_keys"], check=True)
    subprocess.run(["docker", "exec", jump, "install", "-o", "burrow", "-g", "burrow", "-m", "0600",
                    "/tmp/authorized_keys", "/home/burrow/.ssh/authorized_keys"], check=True)
    subprocess.run(["docker", "exec", "-i", jump, "chpasswd"],
                   input=("burrow:" + password + "\n").encode(), check=True)

    image = "burrow/integration-gateway:local"
    subprocess.run(["docker", "build", "-t", image, str(project / "dev/gateway-integration")], check=True)

    socks = "burrow-socks-integration-" + suffix
    subprocess.run(["docker", "create", "--name", socks, *labels,
                    "--publish", "127.0.0.1::1080", "--env", "BURROW_GATEWAY=socks", image],
                   check=True, stdout=subprocess.DEVNULL)
    containers.append(socks)
    subprocess.run(["docker", "cp", str(runtime / "password"), socks + ":/run/socks-password"], check=True)
    subprocess.run(["docker", "start", socks], check=True, stdout=subprocess.DEVNULL)

    def wg(*args, data=None):
        return subprocess.run(["docker", "run", "--rm", "-i", "--entrypoint", "wg", image, *args],
                              input=data, check=True, stdout=subprocess.PIPE, text=True).stdout.strip()
    server_key, client_key = wg("genkey"), wg("genkey")
    server_public, client_public = wg("pubkey", data=server_key), wg("pubkey", data=client_key)
    subnet = subprocess.check_output(
        ["docker", "network", "inspect", "bridge", "--format", "{{(index .IPAM.Config 0).Subnet}}"], text=True).strip()
    server_conf = runtime / "wg-server.conf"
    server_conf.write_text("[Interface]\nPrivateKey = " + server_key + "\nListenPort = 51820\n"
                           "[Peer]\nPublicKey = " + client_public + "\nAllowedIPs = 10.99.0.2/32\n")
    os.chmod(server_conf, 0o600)
    wireguard = "burrow-wireguard-integration-" + suffix
    subprocess.run(["docker", "create", "--name", wireguard, *labels,
                    "--cap-add", "NET_ADMIN", "--sysctl", "net.ipv4.ip_forward=1",
                    "--publish", "127.0.0.1::51820/udp", "--env", "BURROW_GATEWAY=wireguard",
                    "--env", "BURROW_WG_ADDRESS=10.99.0.1/24", image],
                   check=True, stdout=subprocess.DEVNULL)
    containers.append(wireguard)
    subprocess.run(["docker", "cp", str(server_conf), wireguard + ":/run/wg0.conf"], check=True)
    subprocess.run(["docker", "start", wireguard], check=True, stdout=subprocess.DEVNULL)
    endpoint = subprocess.check_output(["docker", "port", wireguard, "51820/udp"], text=True).strip()
    client_conf = runtime / "wg-client.conf"
    client_conf.write_text("[Interface]\nPrivateKey = " + client_key + "\nAddress = 10.99.0.2/32\n"
                           "[Peer]\nPublicKey = " + server_public + "\nEndpoint = " + endpoint + "\n"
                           "AllowedIPs = 10.99.0.0/24, " + subnet + "\n")
    os.chmod(client_conf, 0o600)

    state = dict(
        containers=containers, directory=str(runtime), user="burrow",
        target=dict(container=target["container"], address=container_address(target["container"]), port=22),
        jump=dict(container=jump, internal=container_address(jump),
                  address=subprocess.check_output(["docker", "port", jump, "22/tcp"], text=True).strip()),
        socks=dict(container=socks, internal=container_address(socks), user="burrow",
                   address=subprocess.check_output(["docker", "port", socks, "1080/tcp"], text=True).strip()),
        wireguard=dict(container=wireguard, internal=container_address(wireguard), endpoint=endpoint,
                       config=str(client_conf), tunnel="10.99.0.1"),
    )
    (runtime / "gateway-server.json").write_text(json.dumps(state, indent=2) + "\n")
    for name in ("jump", "socks"):
        host, port = state[name]["address"].rsplit(":", 1)
        deadline = time.monotonic() + 15
        while True:
            try:
                socket.create_connection((host, int(port)), timeout=1).close()
                break
            except OSError:
                if time.monotonic() > deadline:
                    raise RuntimeError(name + " gateway fixture did not become ready")
                time.sleep(0.1)
    deadline = time.monotonic() + 15
    while subprocess.run(["docker", "exec", wireguard, "wg", "show", "wg0", "listen-port"],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
        if time.monotonic() > deadline:
            raise RuntimeError("WireGuard gateway fixture did not become ready; inspect the container logs")
        time.sleep(0.1)
    print(json.dumps(state))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gateways", action="store_true",
                        help="start SSH jump, SOCKS5 and WireGuard gateways beside the BURROW_TEST_DIR SSH fixture")
    parser.add_argument("--ftp", action="store_true", help="start FTP using BURROW_TEST_DIR credentials")
    parser.add_argument("--ftps", action="store_true", help="start FTPS with a disposable trusted CA")
    parser.add_argument("--implicit", action="store_true", help="use implicit FTPS instead of explicit AUTH TLS")
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    if args.ftp and args.ftps or args.implicit and not args.ftps:
        parser.error("use --ftp, --ftps, or --ftps --implicit")
    if args.gateways:
        if args.ftp or args.ftps:
            parser.error("--gateways cannot be combined with --ftp or --ftps")
        start_gateways(project)
        return
    if args.ftp or args.ftps:
        start_ftp(project, "implicit" if args.implicit else "explicit" if args.ftps else "none")
        return
    runtime = Path(tempfile.mkdtemp(prefix="burrow-integration-", dir="/private/tmp"))
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
    image = "burrow/integration-sshd:local"
    subprocess.run(["docker", "build", "-t", image, str(project / "dev/ssh-integration")], check=True)
    name = "burrow-integration-" + secrets.token_hex(4)
    subprocess.run(
        ["docker", "run", "-d", "--name", name, "--label", "dev.burrow.integration=true",
         "--publish", "127.0.0.1::22", image],
        check=True, stdout=subprocess.DEVNULL,
    )
    # Only disposable public keys cross into the local Docker fixture.
    subprocess.run(["docker", "cp", str(authorized), name + ":/tmp/authorized_keys"], check=True)
    subprocess.run(
        ["docker", "exec", name, "install", "-o", "burrow", "-g", "burrow", "-m", "0600",
         "/tmp/authorized_keys", "/home/burrow/.ssh/authorized_keys"], check=True,
    )
    subprocess.run(
        ["docker", "exec", "-i", name, "chpasswd"],
        input=("burrow:" + (runtime / "password").read_text() + "\n").encode(), check=True,
    )
    fixture_files(name)
    address = subprocess.check_output(["docker", "port", name, "22/tcp"], text=True).strip()
    state = {"container": name, "address": address, "directory": str(runtime), "user": "burrow"}
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
