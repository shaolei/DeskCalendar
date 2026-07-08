#!/usr/bin/env python3
# run_spike.py — 启动 spike 并按 demo 时序截图验证双循环 + channel 显隐/定位
import subprocess, time, os, sys, ctypes

HERE = os.path.dirname(os.path.abspath(__file__))
SPIKE = os.path.join(HERE, "..", "spike.exe")
CAP = os.path.join(HERE, "capture.py")
OUT = HERE
PY = "C:/Users/shaolei/.workbuddy/binaries/python/versions/3.13.12/python.exe"

# 绿底壁纸，便于判断"窗口隐藏=透出桌面"
subprocess.run([PY, CAP, "setbg", "00FF00"], check=True)

# 启动 spike，stdout/stderr 落日志
logf = open(os.path.join(OUT, "spike.log"), "w", encoding="utf-8")
p = subprocess.Popen([SPIKE], stdout=logf, stderr=subprocess.STDOUT,
                     cwd=os.path.join(HERE, ".."))
print(f"spike pid={p.pid}")

# 截图时间轴（与 main.go 的 demo 定时器对齐）
# 0s 启动 -> 2s hide -> 4s show -> 6s position -> 8s hide -> 9s quit
timeline = [
    (1.5, "s01_shown"),       # 默认可见
    (3.5, "s02_hidden"),      # 2s 后 hide
    (5.5, "s03_shown"),       # 4s 后 show
    (7.5, "s04_positioned"),  # 6s 后 定位到托盘
    (9.3, "s05_hidden_end"),  # 8s 后 hide
]
t0 = time.monotonic()
for t, name in timeline:
    left = t - (time.monotonic() - t0)
    if left > 0:
        time.sleep(left)
    subprocess.run([PY, CAP, "cap", os.path.join(OUT, name)])
    print(f"captured {name} @ {time.monotonic()-t0:.1f}s")

# 等进程退出（demo 在 ~10s quit）
try:
    p.wait(timeout=8)
    print("spike exited cleanly")
except subprocess.TimeoutExpired:
    print("spike did not exit in time, killing")
    p.kill()

# 恢复壁纸
subprocess.run([PY, CAP, "restorebg"], check=True)
print("done")
