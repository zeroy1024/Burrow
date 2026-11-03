# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///

"""Build native platform artifacts without changing the project's toolchain."""

import argparse
import json
from pathlib import Path
import subprocess
import zipfile


project_root = Path(__file__).resolve().parent.parent


def main():
    parser = argparse.ArgumentParser(description="Build Burrow for macOS, Windows, or Linux")
    parser.add_argument(
        "--platform",
        help="comma-separated targets, e.g. windows/amd64,windows/arm64; defaults to the host",
    )
    args = parser.parse_args()
    host = json.loads(subprocess.check_output(["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH"], text=True))
    requested = args.platform or f"{host['GOHOSTOS']}/{host['GOHOSTARCH']}"
    targets = requested.split(",")
    supported = {f"{system}/{arch}" for system in ("darwin", "windows", "linux") for arch in ("amd64", "arm64")}
    for target in targets:
        if target not in supported:
            parser.error(f"unsupported platform: {target}")
        if target.startswith("darwin/") and host["GOHOSTOS"] != "darwin":
            parser.error("macOS APP/DMG/ZIP packaging requires a macOS host")

    config = json.loads((project_root / "mygo.json").read_text(encoding="utf-8"))
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

        subprocess.run(
            ["go", "tool", "mygo", "build", "-platform", target],
            cwd=project_root,
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
