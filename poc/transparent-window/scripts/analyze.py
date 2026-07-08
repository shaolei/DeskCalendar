#!/usr/bin/env python3
# analyze.py — 对比 baseline 与含窗截图，判定圆角是否透明
#   python analyze.py <baseline.raw> <poc.raw> [out.png]
#
# 判定逻辑（前后对比法，消除"桌面本身是黑"的歧义）：
#   1. 在 poc 图中定位蓝色圆角面板 (近似 #3B6FE0) 的包围盒。
#   2. 在每个圆角的"切角区"（在包围盒内、但在圆角半径 16 之外）取若干采样点。
#   3. 若这些点的颜色与 baseline 同坐标一致 -> 该角透出桌面 -> 透明。
#      若这些点为黑 (0,0,0 附近) -> 窗口不透明实心 -> 失败。
import sys, struct, zlib

PANEL = (59, 111, 224)  # #3B6FE0

def load_raw(path):
    with open(path, "rb") as f:
        w, h = struct.unpack("<II", f.read(8))
        data = f.read()
    return w, h, data

def getpx(data, w, h, x, y):
    i = (y * w + x) * 4
    b, g, r, a = data[i], data[i + 1], data[i + 2], data[i + 3]
    return (r, g, b)

def is_panel(rgb):
    r, g, b = rgb
    return (abs(r - PANEL[0]) < 35 and abs(g - PANEL[1]) < 35 and abs(b - PANEL[2]) < 35
            and b > r and b > g)

def near(a, b, tol=25):
    return all(abs(a[i] - b[i]) < tol for i in range(3))

def main():
    if len(sys.argv) < 3:
        print("usage: analyze.py <baseline.raw> <poc.raw> [out.png]")
        return
    bw, bh, bdata = load_raw(sys.argv[1])
    pw, ph, pdata = load_raw(sys.argv[2])

    # 1) 定位面板包围盒
    minx, miny, maxx, maxy = pw, ph, -1, -1
    for y in range(ph):
        for x in range(pw):
            if is_panel(getpx(pdata, pw, ph, x, y)):
                if x < minx: minx = x
                if x > maxx: maxx = x
                if y < miny: miny = y
                if y > maxy: maxy = y
    pw_w, pw_h = maxx - minx + 1, maxy - miny + 1
    print(f"panel bbox: x[{minx},{maxx}] y[{miny},{maxy}] size {pw_w}x{pw_h}")
    if pw_w < 20 or pw_h < 20:
        print("VERDICT: INCONCLUSIVE — 未检测到蓝色圆角面板，窗口可能未渲染 / GPU 初始化失败")

    # 2) 四角切角区采样
    corners = {
        "TL": [(minx + 18, miny + 18), (minx + 26, miny + 10), (minx + 10, miny + 26)],
        "TR": [(maxx - 18, miny + 18), (maxx - 26, miny + 10), (maxx - 10, miny + 26)],
        "BL": [(minx + 18, maxy - 18), (minx + 26, maxy - 10), (minx + 10, maxy - 26)],
        "BR": [(maxx - 18, maxy - 18), (maxx - 26, maxy - 10), (maxx - 10, maxy - 26)],
    }
    results = {}
    for name, pts in corners.items():
        match = black = 0
        samples = []
        for (x, y) in pts:
            if x < 0 or y < 0 or x >= pw or y >= ph:
                continue
            pc = getpx(pdata, pw, ph, x, y)
            bc = getpx(bdata, bw, bh, x, y)
            samples.append((x, y, pc, bc))
            if near(pc, bc):
                match += 1
            if pc[0] < 20 and pc[1] < 20 and pc[2] < 20:
                black += 1
        results[name] = (match, len(pts), black, samples)
        print(f"{name}: match {match}/{len(pts)}, black {black}  e.g. poc={samples[0][2]} base={samples[0][3]}")

    # 3) 判定
    allmatch = all(m[0] == m[1] and m[1] > 0 for m in results.values())
    anyblack = any(m[2] > 0 for m in results.values())
    if allmatch:
        verdict = "PASS — 圆角外侧透出桌面（每像素 alpha 生效），ADR-03 通过"
    elif anyblack:
        verdict = "FAIL — 圆角外侧为黑色实心（窗口不透明），ADR-03 未通过"
    else:
        verdict = "AMBIGUOUS — 见上方数据，建议手动查看 poc.png"
    print("VERDICT:", verdict)

    # 4) 输出标注 PNG 便于人工核对
    if len(sys.argv) >= 4:
        write_annotated(sys.argv[4] if False else sys.argv[3], pw, ph, pdata,
                        (minx, miny, maxx, maxy), results, verdict)

def write_annotated(path, w, h, data, bbox, results, verdict):
    # 复制为 RGBA 像素，画红框 + 采样点
    out = bytearray()
    stride = w * 4
    for y in range(h):
        out.append(0)
        row = data[y * stride:(y + 1) * stride]
        for x in range(w):
            i = x * 4
            out += bytes((row[i + 2], row[i + 1], row[i + 0], row[i + 3]))
    # 简单画点（黄=采样，红=包围盒角）
    def setpx(x, y, rgb):
        if 0 <= x < w and 0 <= y < h:
            o = (y * w + x) * 4 + 1  # after filter byte per row
            # 注意：out 是带 filter 字节的，需按行偏移
            pass
    # 为避免行偏移出错，直接重写一个干净的 RGBA 缓冲
    px = bytearray(w * h * 4)
    for y in range(h):
        for x in range(w):
            i = (y * w + x) * 4
            j = (y * w + x) * 4
            px[j] = data[i + 2]; px[j + 1] = data[i + 1]; px[j + 2] = data[i + 0]; px[j + 3] = 255
    def dot(x, y, col):
        for dy in range(-3, 4):
            for dx in range(-3, 4):
                xx, yy = x + dx, y + dy
                if 0 <= xx < w and 0 <= yy < h:
                    o = (yy * w + xx) * 4
                    px[o] = col[0]; px[o + 1] = col[1]; px[o + 2] = col[2]
    minx, miny, maxx, maxy = bbox
    # 包围盒四角红点
    for (cx, cy) in [(minx, miny), (maxx, miny), (minx, maxy), (maxx, maxy)]:
        dot(cx, cy, (255, 0, 0))
    # 采样点黄点
    for name, (match, total, black, samples) in results.items():
        for (x, y, pc, bc) in samples:
            dot(x, y, (255, 220, 0))
    raw = bytearray()
    for y in range(h):
        raw.append(0)
        raw += px[y * w * 4:(y + 1) * w * 4]
    comp = zlib.compress(bytes(raw), 9)
    def chunk(typ, body):
        return struct.pack(">I", len(body)) + typ + body + struct.pack(">I", zlib.crc32(typ + body) & 0xffffffff)
    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n")
        ihdr = struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0)
        f.write(chunk(b"IHDR", ihdr))
        f.write(chunk(b"IDAT", comp))
        f.write(chunk(b"IEND", b""))

if __name__ == "__main__":
    main()
