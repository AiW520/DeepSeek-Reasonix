#!/usr/bin/env python3
"""Generate deterministic desktop icon assets from a square raster source."""

from __future__ import annotations

import argparse
import base64
import io
import struct
from pathlib import Path

from PIL import Image, ImageDraw, ImageEnhance, ImageFilter


SIZES = (16, 24, 32, 48, 64, 128, 256, 512)


def rounded_icon(source: Image.Image, size: int, inset: int = 0) -> Image.Image:
    canvas = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    visible = size - inset * 2
    artwork = source.resize((visible, visible), Image.Resampling.LANCZOS)
    artwork = ImageEnhance.Contrast(artwork).enhance(1.04)
    artwork = ImageEnhance.Color(artwork).enhance(1.03)

    radius = max(2, round(visible * 0.195))
    mask = Image.new("L", (visible, visible), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, visible - 1, visible - 1), radius=radius, fill=255)
    if size <= 48:
        artwork = artwork.filter(ImageFilter.UnsharpMask(radius=0.7, percent=135, threshold=2))
    canvas.paste(artwork, (inset, inset), mask)
    return canvas


def png_bytes(image: Image.Image) -> bytes:
    buffer = io.BytesIO()
    image.save(buffer, format="PNG", optimize=True)
    return buffer.getvalue()


def write_ico(path: Path, source: Image.Image) -> None:
    sizes = (16, 24, 32, 48, 64, 256)
    payloads = [png_bytes(rounded_icon(source, size)) for size in sizes]
    header_size = 6 + 16 * len(sizes)
    offset = header_size
    entries = []
    for size, payload in zip(sizes, payloads):
        dimension = 0 if size == 256 else size
        entries.append(struct.pack("<BBBBHHII", dimension, dimension, 0, 0, 1, 32, len(payload), offset))
        offset += len(payload)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(struct.pack("<HHH", 0, 1, len(sizes)) + b"".join(entries) + b"".join(payloads))


def write_icns(path: Path, source: Image.Image) -> None:
    chunks = []
    for kind, size, inset in ((b"ic07", 128, 13), (b"ic08", 256, 25), (b"ic09", 512, 50), (b"ic10", 1024, 100)):
        payload = png_bytes(rounded_icon(source, size, inset))
        chunks.append(kind + struct.pack(">I", len(payload) + 8) + payload)
    body = b"".join(chunks)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(b"icns" + struct.pack(">I", len(body) + 8) + body)


def write_svg(path: Path, source: Image.Image) -> None:
    preview = source.resize((512, 512), Image.Resampling.LANCZOS).convert("RGB")
    buffer = io.BytesIO()
    preview.save(buffer, format="JPEG", quality=90, optimize=True, progressive=True)
    encoded = base64.b64encode(buffer.getvalue()).decode("ascii")
    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">
  <defs><clipPath id="icon"><rect width="1024" height="1024" rx="200"/></clipPath></defs>
  <image width="1024" height="1024" clip-path="url(#icon)" preserveAspectRatio="xMidYMid slice" href="data:image/jpeg;base64,{encoded}"/>
</svg>
'''
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(svg, encoding="ascii", newline="\n")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("--desktop", type=Path, default=Path("desktop"))
    args = parser.parse_args()

    with Image.open(args.source) as raw:
        source = raw.convert("RGBA")
    if source.width != source.height:
        side = min(source.size)
        left = (source.width - side) // 2
        top = (source.height - side) // 2
        source = source.crop((left, top, left + side, top + side))

    build = args.desktop / "build"
    rounded_icon(source, 1024).save(build / "appicon.png", format="PNG", optimize=True)
    write_ico(build / "windows" / "icon.ico", source)
    write_icns(build / "darwin" / "icon.icns", source)
    write_svg(build / "appicon.svg", source)
    write_svg(build / "darwin" / "appicon.svg", source)

    linux = build / "linux" / "icons" / "hicolor"
    for size in SIZES:
        target = linux / f"{size}x{size}" / "apps" / "reasonix-desktop.png"
        target.parent.mkdir(parents=True, exist_ok=True)
        rounded_icon(source, size).save(target, format="PNG", optimize=True)
    write_svg(linux / "scalable" / "apps" / "reasonix-desktop.svg", source)


if __name__ == "__main__":
    main()
