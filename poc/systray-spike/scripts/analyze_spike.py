#!/usr/bin/env python3
# analyze_spike.py — 量化每张截图里蓝色面板(0x1e63d8)的存在/位置
# 蓝面板 RGB≈(30,99,216)；判定像素: R<90 且 G∈[60,150] 且 B>150 且 B>R+80
import struct, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))

def load_raw(path):
    with open(path, "rb") as f:
        w, h = struct.unpack("<II", f.read(8))
        data = f.read()
    return w, h, data  # BGRA

def panel_bbox(data, w, h):
    minx = miny = 10**9
    maxx = maxy = -1
    cnt = 0
    stride = w * 4
    for y in range(0, h, 2):  # 抽样提速
        row = data[y*stride:(y+1)*stride]
        for x in range(0, w, 2):
            i = x*4
            b, g, r = row[i], row[i+1], row[i+2]
            if r < 90 and 60 <= g <= 150 and b > 150 and b - r > 80:
                cnt += 1
                if x < minx: minx = x
                if x > maxx: maxx = x
                if y < miny: miny = y
                if y > maxy: maxy = y
    if cnt == 0:
        return None, 0
    return (minx*2, miny*2, maxx*2, maxy*2), cnt*4  # 还原抽样倍率

names = ["s01_shown", "s02_hidden", "s03_shown", "s04_positioned", "s05_hidden_end"]
print(f"{'stage':16} {'panel?':8} {'bbox(x0,y0,x1,y1)':24} {'blue_px':>10}")
for n in names:
    raw = os.path.join(HERE, n + ".raw")
    if not os.path.exists(raw):
        print(f"{n:16} MISSING")
        continue
    w, h, data = load_raw(raw)
    bbox, cnt = panel_bbox(data, w, h)
    if bbox is None:
        print(f"{n:16} {'NO':8} {'-':24} {0:>10}")
    else:
        print(f"{n:16} {'YES':8} {str(bbox):24} {cnt:>10}")
