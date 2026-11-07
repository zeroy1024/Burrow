# Native README gallery build

[English](README.md) | [简体中文](README.zh.md)

Build a separate `Burrow Gallery.app` with current components and static mock data for README screenshots. [scene.go.txt](scene.go.txt) intentionally avoids `.go`; only [prepare.py](prepare.py) includes it through a temporary Go overlay, outside ordinary builds/tests.

See [screenshot notes](../../docs/screenshots/readme/README.md) for commands, scene shortcuts, and data boundaries. Temporary builds/SDK settings remain under the script's printed `/private/tmp/burrow-readme-gallery-*` directory for reproduction. Existing Burrow installs and real connection settings are not replaced/read.
