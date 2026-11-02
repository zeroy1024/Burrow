# /// script
# requires-python = ">=3.10"
# dependencies = ["dmgbuild==1.6.7"]
# ///

import json
from pathlib import Path
import subprocess

import dmgbuild


project_root = Path(__file__).resolve().parent.parent
config = json.loads((project_root / "mygo.json").read_text(encoding="utf-8"))
arch = subprocess.check_output(["go", "env", "GOARCH"], text=True).strip()

subprocess.run(
    ["go", "tool", "mygo", "build", "-skip-dmg", "-platform", f"darwin/{arch}"],
    cwd=project_root,
    check=True,
)

output_dir = project_root / config["out"] / f"darwin-{arch}"
app_path = output_dir / f"{config['name']}.app"
artifact_name = f"{config['slug']}-{config['version']}-{arch}"
dmg_path = output_dir / f"{artifact_name}.dmg"
dmgbuild.build_dmg(
    str(dmg_path),
    config["name"],
    settings={
        "format": "ULMO",
        "files": [str(app_path)],
        "symlinks": {"Applications": "/Applications"},
        "icon": str(project_root / config["macos"]["icon"]),
        # The TIFF contains both 1x and 2x images; Finder coordinates use points.
        "background": str(project_root / "assets/dmg/background.tiff"),
        "window_rect": ((200, 400), (660, 400)),
        "icon_size": 128,
        "text_size": 13,
        "icon_locations": {app_path.name: (170, 180), "Applications": (490, 180)},
    },
)
print(f"Created {dmg_path.relative_to(project_root)}", flush=True)

zip_path = output_dir / f"{artifact_name}.zip"
subprocess.run(
    ["ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", str(app_path), str(zip_path)],
    check=True,
)
print(f"Created {zip_path.relative_to(project_root)}")
