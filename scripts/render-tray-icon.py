# /// script
# requires-python = ">=3.10"
# dependencies = ["resvg-py==0.5.0"]
# ///

"""Render the macOS menu bar template at four pixels per point."""

from pathlib import Path

import resvg_py


def main():
    assets = Path(__file__).resolve().parent.parent / "assets"
    image = resvg_py.svg_to_bytes(
        svg_path=str(assets / "tray-template.svg"),
        width=72,
        height=72,
    )
    (assets / "tray-template.png").write_bytes(image)


if __name__ == "__main__":
    main()
