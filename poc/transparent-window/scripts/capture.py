#!/usr/bin/env python3
# capture.py — 纯 stdlib 截屏工具（无 pip 依赖）
# 用途：
#   python capture.py cap <name>      截全屏 -> <name>.raw (分析用) + <name>.png (查看用)
#   python capture.py setbg <hex>      把桌面壁纸临时设为指定纯色（消除"桌面本身是黑"的歧义）
#   python capture.py restorebg        恢复截屏前保存的原始壁纸
#
# 截屏用 user32.GetDC(0) + gdi32.BitBlt 抓整屏合成结果（含 WS_EX_LAYERED 透明窗）。
import sys, ctypes, zlib, struct, os, winreg

user32 = ctypes.windll.user32
gdi32 = ctypes.windll.gdi32
SRCCOPY = 0x00CC0020

class BITMAPINFOHEADER(ctypes.Structure):
    _fields_ = [
        ("biSize", ctypes.c_uint32), ("biWidth", ctypes.c_int32), ("biHeight", ctypes.c_int32),
        ("biPlanes", ctypes.c_uint16), ("biBitCount", ctypes.c_uint16), ("biCompression", ctypes.c_uint32),
        ("biSizeImage", ctypes.c_uint32), ("biXPelsPerMeter", ctypes.c_int32), ("biYPelsPerMeter", ctypes.c_int32),
        ("biClrUsed", ctypes.c_uint32), ("biClrImportant", ctypes.c_uint32),
    ]

class BITMAPINFO(ctypes.Structure):
    _fields_ = [("bmiHeader", BITMAPINFOHEADER), ("bmiColors", ctypes.c_uint32 * 3)]

SAVED_BG = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".saved_wallpaper.txt")

def capture():
    w = user32.GetSystemMetrics(0)   # SM_CXSCREEN
    h = user32.GetSystemMetrics(1)   # SM_CYSCREEN
    hdc = user32.GetDC(0)
    hdcMem = gdi32.CreateCompatibleDC(hdc)
    bmp = gdi32.CreateCompatibleBitmap(hdc, w, h)
    gdi32.SelectObject(hdcMem, bmp)
    gdi32.BitBlt(hdcMem, 0, 0, w, h, hdc, 0, 0, SRCCOPY)
    buf = ctypes.create_string_buffer(w * h * 4)
    bmi = BITMAPINFO()
    bmi.bmiHeader.biSize = ctypes.sizeof(BITMAPINFOHEADER)
    bmi.bmiHeader.biWidth = w
    bmi.bmiHeader.biHeight = -h  # top-down
    bmi.bmiHeader.biPlanes = 1
    bmi.bmiHeader.biBitCount = 32
    bmi.bmiHeader.biCompression = 0
    gdi32.GetDIBits(hdcMem, bmp, 0, h, buf, ctypes.byref(bmi), 0)
    gdi32.DeleteObject(bmp)
    gdi32.DeleteDC(hdcMem)
    user32.ReleaseDC(0, hdc)
    return w, h, buf.raw  # BGRA, top-down

def write_raw(path, w, h, data):
    with open(path, "wb") as f:
        f.write(struct.pack("<II", w, h))
        f.write(data)

def write_png(path, w, h, data):
    raw = bytearray()
    stride = w * 4
    for y in range(h):
        raw.append(0)  # filter type 0
        row = data[y * stride:(y + 1) * stride]
        for x in range(w):
            i = x * 4
            raw += bytes((row[i + 2], row[i + 1], row[i + 0], row[i + 3]))  # BGRA->RGBA
    comp = zlib.compress(bytes(raw), 9)

    def chunk(typ, body):
        return struct.pack(">I", len(body)) + typ + body + struct.pack(">I", zlib.crc32(typ + body) & 0xffffffff)

    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n")
        ihdr = struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)
        f.write(chunk(b"IHDR", ihdr))
        f.write(chunk(b"IDAT", comp))
        f.write(chunk(b"IEND", b""))

def get_wallpaper():
    try:
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Control Panel\Desktop")
        val, _ = winreg.QueryValueEx(key, "WallPaper")
        return val
    except Exception as e:
        return ""

def set_wallpaper(path):
    user32.SystemParametersInfoW.argtypes = [ctypes.c_uint, ctypes.c_uint, ctypes.c_void_p, ctypes.c_uint]
    user32.SystemParametersInfoW(20, 0, ctypes.c_wchar_p(path), 3)  # SPI_SETDESKWALLPAPER

def make_solid_bmp(path, r, g, b, w=1920, h=1080):
    # 简单 24bpp BMP，纯色
    row_bytes = (w * 3 + 3) & ~3
    pix = bytearray()
    for _ in range(h):
        row = bytes((b, g, r)) * w
        pix += row
        pix += b"\x00" * (row_bytes - len(row))
    filesize = 54 + len(pix)
    with open(path, "wb") as f:
        f.write(b"BM")
        f.write(struct.pack("<I", filesize))
        f.write(b"\x00\x00\x00\x00")
        f.write(struct.pack("<I", 54))
        f.write(struct.pack("<I", 40))
        f.write(struct.pack("<i", w))
        f.write(struct.pack("<i", -h))
        f.write(struct.pack("<H", 1))
        f.write(struct.pack("<H", 24))
        f.write(struct.pack("<I", 0))
        f.write(struct.pack("<I", len(pix)))
        f.write(struct.pack("<i", 2835))
        f.write(struct.pack("<i", 2835))
        f.write(struct.pack("<I", 0))
        f.write(struct.pack("<I", 0))
        f.write(pix)

if __name__ == "__main__":
    if len(sys.argv) >= 3 and sys.argv[1] == "cap":
        name = sys.argv[2]
        w, h, data = capture()
        write_raw(name + ".raw", w, h, data)
        write_png(name + ".png", w, h, data)
        print(f"captured {w}x{h} -> {name}.raw / {name}.png")
    elif len(sys.argv) >= 2 and sys.argv[1] == "setbg":
        hexc = sys.argv[2] if len(sys.argv) >= 3 else "00FF00"
        r = int(hexc[0:2], 16); g = int(hexc[2:4], 16); b = int(hexc[4:6], 16)
        cur = get_wallpaper()
        with open(SAVED_BG, "w") as f:
            f.write(cur or "")
        bg_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".solid_bg.bmp")
        make_solid_bmp(bg_path, r, g, b)
        set_wallpaper(bg_path)
        print(f"wallpaper set to #{hexc} (saved previous: {cur!r})")
    elif sys.argv[1] == "restorebg":
        prev = ""
        if os.path.exists(SAVED_BG):
            with open(SAVED_BG) as f:
                prev = f.read().strip()
        set_wallpaper(prev)
        print(f"wallpaper restored -> {prev!r}")
    else:
        print("usage:")
        print("  capture.py cap <name>")
        print("  capture.py setbg [RRGGBB]")
        print("  capture.py restorebg")
