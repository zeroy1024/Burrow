#!/usr/bin/env python3
"""Run the pre-push E2E gate against disposable local Docker fixtures.

Layers: protocol (Go integration tests), native (macOS UI driver) and mcp
(native MCP scenarios). Each layer starts its own fixtures and removes the
containers it started. A skipped integration test fails the gate.
"""

import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import subprocess
import sys
import time


REPO = Path(__file__).resolve().parents[1]
SERVER = REPO / "scripts/ssh-integration-server.py"
FIXTURES = dict(ftp=["--ftp"], ftps=["--ftps"], ftps_implicit=["--ftps", "--implicit"], gateways=["--gateways"])
# Needs a part file reported from a real interrupted transfer; no fixture creates one.
ALLOWED_SKIPS = {"TestFTPIntegrationReportedPrefix"}


def start_fixture(args, env):
    output = subprocess.run([sys.executable, "-B", str(SERVER), *args], env=env, check=True,
                            stdout=subprocess.PIPE, text=True).stdout
    return json.loads(output.strip().splitlines()[-1])


@contextmanager
def fixtures(kinds):
    containers = []
    try:
        ssh = start_fixture([], os.environ)
        containers.append(ssh["container"])
        env = dict(os.environ, BURROW_TEST_ADDRESS=ssh["address"], BURROW_TEST_DIR=ssh["directory"])
        for kind in kinds:
            state = start_fixture(FIXTURES[kind], env)
            containers.extend(state.get("containers") or [state["container"]])
            if kind == "ftp":
                env["BURROW_FTP_ADDRESS"] = state["address"]
        print("Fixture directory: " + ssh["directory"], flush=True)
        yield env
    finally:
        if containers:
            # Gateways of a launcher that failed part-way carry their SSH fixture's name.
            containers += subprocess.run(
                ["docker", "ps", "--all", "--format", "{{.Names}}", "--filter", "label=dev.burrow.gateway=" + containers[0]],
                stdout=subprocess.PIPE, text=True, check=True).stdout.split()
        for container in dict.fromkeys(containers):
            subprocess.run(["docker", "rm", "--force", container], stdout=subprocess.DEVNULL, check=True)


def protocol():
    with fixtures(("ftp", "ftps", "ftps_implicit", "gateways")) as env:
        process = subprocess.Popen(
            ["go", "test", "-race", "-tags=integration", "-json", "./...", "-count=1", "-timeout=180s"],
            cwd=REPO, env=env, stdout=subprocess.PIPE, text=True)
        output, skipped, finished = {}, [], set()
        for line in process.stdout:
            if not line.startswith("{"):
                print(line, end="", flush=True)
                continue
            event = json.loads(line)
            test, action = event.get("Test"), event["Action"]
            if test is None:
                # Package results and build errors.
                if "Output" in event:
                    print(event["Output"], end="", flush=True)
                if action == "fail":
                    # A panic or race report ends the process inside a test that never reports itself.
                    for key in [key for key in output if key[0] == event.get("Package") and key not in finished]:
                        print("".join(output[key]), end="", flush=True)
                continue
            key = (event["Package"], test)
            if action == "output":
                output.setdefault(key, []).append(event["Output"])
            elif action in ("pass", "skip", "fail"):
                finished.add(key)
            if action == "fail":
                print("".join(output.get(key, [])), end="", flush=True)
            elif action == "skip" and test.split("/")[0] not in ALLOWED_SKIPS:
                skipped.append(key)
        if process.wait():
            raise RuntimeError("protocol integration tests failed")
        if skipped:
            for key in skipped:
                print("".join(output[key]), end="")
            raise RuntimeError("skipped tests do not pass the gate: "
                               + ", ".join(package + "." + test for package, test in skipped))


def require_macos(layer):
    if sys.platform != "darwin":
        raise RuntimeError(layer + " layer drives a native macOS window and cannot run on " + sys.platform)


def build_template():
    subprocess.run(["go", "tool", "mygo", "build", "-skip-dmg"], cwd=REPO, check=True)


def native():
    require_macos("native")
    build_template()
    with fixtures(("ftp", "gateways")) as env:
        for mode in ([], ["--concurrent-split"], ["--file-protocols"], ["--core-e2e"], ["--coverage"]):
            subprocess.run([sys.executable, "-B", "-u", str(REPO / "scripts/ssh-integration-ui.py"), *mode],
                           cwd=REPO, env=env, check=True)


def mcp():
    require_macos("mcp")
    build_template()
    with fixtures(("ftp", "ftps", "ftps_implicit")) as env:
        subprocess.run([sys.executable, "-B", "-u", str(REPO / "scripts/mcp-integration-ui.py")],
                       cwd=REPO, env=env, check=True)


LAYERS = dict(protocol=protocol, native=native, mcp=mcp)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("layers", nargs="*", metavar="layer",
                        help="protocol, native, mcp or all, run in order; default: protocol")
    args = parser.parse_args()
    unknown = set(args.layers) - {*LAYERS, "all"}
    if unknown:
        parser.error("unknown layer: " + ", ".join(sorted(unknown)))
    layers = list(LAYERS) if "all" in args.layers else list(dict.fromkeys(args.layers or ["protocol"]))
    for layer in layers:
        started = time.monotonic()
        print("== gate: " + layer, flush=True)
        try:
            LAYERS[layer]()
        except (RuntimeError, subprocess.CalledProcessError) as error:
            print("FAIL gate " + layer + ": " + str(error), file=sys.stderr)
            sys.exit(1)
        print("PASS gate " + layer + " in " + str(round(time.monotonic() - started)) + "s", flush=True)


if __name__ == "__main__":
    main()
