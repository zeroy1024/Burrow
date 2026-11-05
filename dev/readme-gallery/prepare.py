#!/usr/bin/env python3
"""Build an isolated native gallery using the current Burrow UI and mock data."""

import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import tempfile


repo = Path(__file__).resolve().parents[2]
work = Path(tempfile.mkdtemp(prefix="burrow-readme-gallery-", dir="/private/tmp"))
screenshots = repo / "docs/screenshots/readme"
screenshots.mkdir(parents=True, exist_ok=True)
module = json.loads(subprocess.check_output(
    ["go", "list", "-m", "-json", "github.com/egoist/mygo"], cwd=repo, text=True,
))
sdk = work / "mygo"
shutil.copytree(module["Dir"], sdk)
sdk.chmod(0o700)
paths = sdk / "paths.go"
paths.chmod(0o600)
content = paths.read_text()
assert content.count("os.UserConfigDir()") == 4, "Review SDK configuration isolation"
content = content.replace("os.UserConfigDir()", "galleryUserConfigDir()")
content += "\nfunc galleryUserConfigDir() (string, error) { return " + json.dumps(str(work / "sdk-config")) + ", nil }\n"
paths.write_text(content)
modfile = (repo / "go.mod").read_text()
for module_name, directory in [
    ("github.com/egoist/mygo/sshdesk-terminal", "third_party/mygo-terminal"),
    ("github.com/pkg/sftp", "third_party/pkg-sftp"),
]:
    modfile = modfile.replace(
        f"replace {module_name} => ./{directory}",
        f"replace {module_name} => " + json.dumps(str(repo / directory)),
    )
modfile += "\nreplace github.com/egoist/mygo => " + json.dumps(str(sdk)) + "\n"
(work / "gallery.mod").write_text(modfile)
shutil.copyfile(repo / "go.sum", work / "gallery.sum")
original = repo / "internal/desktop/app.go"
replacement = work / "app.go"
content = original.read_text()
assert content.count("func Run() (runErr error) {") == 1
replacement.write_text(content.replace(
    "func Run() (runErr error) {", "func Run() (runErr error) {\n return runREADMEGallery()", 1,
))
scene = work / "gallery.go"
scene.write_text((Path(__file__).parent / "scene.go.txt").read_text() +
                 "\nconst galleryScreenshotDirectory = " + json.dumps(str(screenshots)) + "\n")
overlay = work / "overlay.json"
overlay.write_text(json.dumps({"Replace": {
    str(original): str(replacement),
    str(repo / "internal/desktop/readme_gallery.go"): str(scene),
}}))
subprocess.run(["gofmt", "-w", str(scene), str(replacement), str(paths)], check=True)
binary = work / "Burrow"
environment = os.environ.copy()
environment.setdefault("GOCACHE", "/private/tmp/sshdesk-go-build")
subprocess.run([
    "go", "build", "-modfile", str(work / "gallery.mod"), "-overlay", str(overlay),
    "-o", str(binary), ".",
], cwd=repo, env=environment, check=True)
template = repo / "build/darwin-arm64/Burrow.app"
bundle = work / "Burrow Gallery.app"
shutil.copytree(template, bundle)
info_path = bundle / "Contents/Info.plist"
info = plistlib.loads(info_path.read_bytes())
info.update(CFBundleIdentifier="dev.burrow.app.gallery", CFBundleName="Burrow Gallery", CFBundleDisplayName="Burrow Gallery")
info_path.write_bytes(plistlib.dumps(info))
shutil.copy2(binary, bundle / "Contents/MacOS" / info["CFBundleExecutable"])
subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(bundle)], check=True)
print(bundle, flush=True)
