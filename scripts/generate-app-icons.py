"""Rasterize the existing shield geometry for Android and Apple app icons.

Uses only the standard library; the paths and colors match web/public/icon.svg.
"""
import math
import struct
import zlib
from pathlib import Path


def curve(a, b, c, d):
    return [tuple((1-t)**3*a[i]+3*(1-t)**2*t*b[i]+3*(1-t)*t*t*c[i]+t**3*d[i] for i in (0, 1)) for t in [j/40 for j in range(41)]]


def icon(size):
    scale = size * 3 / 64
    side = size * 3
    pixels = bytearray([46, 48, 40] * side * side)

    def polygon(points, color):
        points = [(x*scale, y*scale) for x, y in points]
        for y in range(max(0, math.floor(min(p[1] for p in points))), min(side, math.ceil(max(p[1] for p in points)))):
            crossings = []
            for a, b in zip(points, points[1:]+points[:1]):
                if min(a[1], b[1]) <= y+.5 < max(a[1], b[1]):
                    crossings.append(a[0]+(y+.5-a[1])*(b[0]-a[0])/(b[1]-a[1]))
            crossings.sort()
            for left, right in zip(crossings[::2], crossings[1::2]):
                for x in range(max(0, math.ceil(left-.5)), min(side, math.ceil(right-.5))):
                    index = (y*side+x)*3
                    pixels[index:index+3] = bytes(color)

    shield = [(32, 8), (52, 16), (52, 33)] + curve((52, 33), (52, 43), (42, 51), (32, 56)) + curve((32, 56), (22, 51), (12, 43), (12, 33)) + [(12, 16)]
    polygon(shield, (229, 173, 50))
    for points in [[(29, 17), (35, 17), (35, 42), (29, 42)], [(21, 25), (43, 25), (43, 31), (21, 31)]]:
        polygon(points, (46, 48, 40))
    rows = bytearray()
    for y in range(size):
        rows.append(0)
        for x in range(size):
            for channel in range(3):
                rows.append(sum(pixels[((y*3+dy)*side+x*3+dx)*3+channel] for dy in range(3) for dx in range(3))//9)

    def chunk(kind, data):
        return struct.pack('!I', len(data))+kind+data+struct.pack('!I', zlib.crc32(kind+data))
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR', struct.pack('!2I5B', size, size, 8, 2, 0, 0, 0))+chunk(b'IDAT', zlib.compress(rows))+chunk(b'IEND', b'')


if __name__ == '__main__':
    root = Path(__file__).resolve().parents[1] / 'web' / 'public'
    for size in (180, 192, 512):
        (root / f'app-icon-{size}.png').write_bytes(icon(size))
