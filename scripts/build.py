# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///

"""Build native platform artifacts without changing the project's toolchain."""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile


project_root = Path(__file__).resolve().parent.parent


def windows_environment(host, directory):
    upx = shutil.which("upx")
    if upx is None:
        raise FileNotFoundError("Windows packaging requires UPX 5.3.0 or newer on PATH")

    go_name = "go.exe" if host["GOHOSTOS"] == "windows" else "go"
    go = str(Path(host["GOROOT"]) / "bin" / go_name)
    proxy = Path(directory) / go_name
    environment = os.environ.copy()
    environment.update(GOOS=host["GOHOSTOS"], GOARCH=host["GOHOSTARCH"], CGO_ENABLED="0")
    subprocess.run(
        [go, "build", "-trimpath", "-o", str(proxy), "./scripts/upx-go"],
        cwd=project_root,
        env=environment,
        check=True,
    )
    # Run the resolved tool directly: go tool prepends GOROOT/bin to PATH.
    mygo = subprocess.check_output(
        [go, "tool", "-n", "mygo"], cwd=project_root, env=environment, text=True,
    ).strip()
    environment = os.environ.copy()
    environment.update(BURROW_BUILD_GO=go, BURROW_BUILD_UPX=upx)
    environment["PATH"] = str(directory) + os.pathsep + environment["PATH"]
    return environment, mygo


def main():
    parser = argparse.ArgumentParser(description="Build Burrow for macOS, Windows, or Linux")
    parser.add_argument(
        "--platform",
        help="comma-separated targets, e.g. windows/amd64,windows/arm64; defaults to the host",
    )
    args = parser.parse_args()
    host = json.loads(subprocess.check_output(["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH", "GOROOT"], text=True))
    requested = args.platform or f"{host['GOHOSTOS']}/{host['GOHOSTARCH']}"
    targets = requested.split(",")
    supported = {f"{system}/{arch}" for system in ("darwin", "windows", "linux") for arch in ("amd64", "arm64")}
    for target in targets:
        if target not in supported:
            parser.error(f"unsupported platform: {target}")
        if target.startswith("darwin/") and host["GOHOSTOS"] != "darwin":
            parser.error("macOS APP/DMG/ZIP packaging requires a macOS host")

    config = json.loads((project_root / "mygo.json").read_text(encoding="utf-8"))
    with tempfile.TemporaryDirectory(prefix="burrow-build-") as directory:
        windows_env = None
        windows_mygo = None
        if any(target.startswith("windows/") for target in targets):
            windows_env, windows_mygo = windows_environment(host, directory)
        for target in targets:
            system, arch = target.split("/")
            if system == "darwin":
                # Keep the existing signed APP and custom DMG/ZIP delivery intact.
                subprocess.run(
                    ["uv", "run", "--script", "scripts/build-macos.py", "--arch", arch],
                    cwd=project_root,
                    check=True,
                )
                continue

            mygo = [windows_mygo] if system == "windows" else ["go", "tool", "mygo"]
            subprocess.run(
                [*mygo, "build", "-platform", target],
                cwd=project_root,
                env=windows_env if system == "windows" else None,
                check=True,
            )
            if system != "windows":
                continue

            output = project_root / config["out"] / f"windows-{arch}"
            executable = output / f"{config['name']}.exe"
            library = output / "ghostty-vt.dll"
            for resource in (executable, library):
                if not resource.is_file():
                    raise FileNotFoundError(f"Windows runtime artifact missing: {resource}")

            installer = output / f"{config['slug']}-setup-{config['version']}-{arch}.exe"
            portable = output / f"{config['slug']}-{config['version']}-windows-{arch}.zip"
            # MyGo replaces its output directory for every build. Archive this
            # build's executable and resources together, excluding the installer.
            contents = [entry for entry in output.iterdir() if entry != installer]
            with zipfile.ZipFile(portable, "w", compression=zipfile.ZIP_DEFLATED) as archive:
                for entry in contents:
                    files = entry.rglob("*") if entry.is_dir() else (entry,)
                    for file in files:
                        if file.is_file():
                            archive.write(file, Path(config["name"]) / file.relative_to(output))
            print(f"Created {portable.relative_to(project_root)}", flush=True)
            if installer.is_file():
                print(f"Created {installer.relative_to(project_root)}", flush=True)
            else:
                print("Windows installer was not generated: NSIS (makensis) is required on this host.", flush=True)


if __name__ == "__main__":
    main()
