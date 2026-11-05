# Makes test images: a real 2x2 PNG, and a "bomb" whose header claims 100000x100000 pixels.
# usage: python mkpng.py <out-dir>
import struct, sys, zlib, os

out = sys.argv[1]

def chunk(kind, data):
    c = struct.pack(">I", len(data)) + kind + data
    return c + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)

def png(w, h, pixels=True):
    sig = b"\x89PNG\r\n\x1a\n"
    ihdr = chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0))  # 8-bit RGB
    body = b""
    if pixels:
        raw = b"".join(b"\x00" + b"\xff\x00\x00" * w for _ in range(h))  # red, filter 0
        body = chunk(b"IDAT", zlib.compress(raw))
    return sig + ihdr + body + chunk(b"IEND", b"")

open(os.path.join(out, "small.png"), "wb").write(png(2, 2))
open(os.path.join(out, "small2.png"), "wb").write(png(3, 3))
open(os.path.join(out, "bomb.png"), "wb").write(png(100000, 100000, pixels=False))
print("ok")
