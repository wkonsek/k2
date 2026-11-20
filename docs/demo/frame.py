#!/usr/bin/env python3
import sys
from PIL import Image, ImageDraw, ImageFont, ImageSequence

LINE = 2
BAR = 26
GRIP = 4
CHROME = LINE + BAR + GRIP
TAB_BG = (14, 23, 17)
ACTIVE = (34, 180, 85)
TEXT = (198, 232, 208)
MUTED = (109, 111, 124)
GRIP_C = (7, 11, 8)
BTN_QUIT = (255, 41, 59)
FONT_PATH = "/usr/share/fonts/truetype/sand-box/google/Geist/Geist-VariableFont_wght.ttf"


def font(size):
    return ImageFont.truetype(FONT_PATH, size=size)


def tab_width(draw, label, f, pad=22):
    box = draw.textbbox((0, 0), label, font=f)
    return box[2] - box[0] + pad


def draw_chrome(base):
    w, h = base.size
    out = Image.new("RGBA", (w, h + CHROME), TAB_BG + (255,))
    out.paste(base.convert("RGBA"), (0, 0))
    d = ImageDraw.Draw(out)
    y0 = h
    d.rectangle([0, y0, w, y0 + LINE - 1], fill=ACTIVE)
    d.rectangle([0, y0 + LINE, w, y0 + LINE + BAR - 1], fill=TAB_BG)
    d.rectangle([0, y0 + LINE + BAR, w, y0 + CHROME - 1], fill=GRIP_C)
    f = font(13)
    tabs = [("k2 - kind-dev", True), ("shell", False), ("logs", False)]
    x = 16
    tab_y = y0 + LINE
    for label, on in tabs:
        tw = tab_width(d, label, f)
        if on:
            d.rectangle([x, tab_y, x + tw, tab_y + BAR - 1], fill=ACTIVE)
        color = TEXT if on else MUTED
        bbox = d.textbbox((0, 0), label, font=f)
        th = bbox[3] - bbox[1]
        tx = x + (tw - (bbox[2] - bbox[0])) // 2
        ty = tab_y + (BAR - th) // 2 - bbox[1]
        d.text((tx, ty), label, font=f, fill=color)
        x += tw + 2
    btn = 10
    gap = 8
    by = tab_y + (BAR - btn) // 2
    qx = w - 16 - btn
    d.rounded_rectangle([qx, by, qx + btn, by + btn], radius=1, fill=BTN_QUIT)
    cx = qx - gap - btn
    d.rounded_rectangle([cx, by, cx + btn, by + btn], radius=1, fill=ACTIVE)
    return out.convert("RGB")


def frame_gif(path):
    src = Image.open(path)
    frames = []
    durations = []
    for im in ImageSequence.Iterator(src):
        frames.append(draw_chrome(im.convert("RGB")))
        durations.append(im.info.get("duration", 60))
    loop = src.info.get("loop", 0)
    frames[0].save(
        path,
        save_all=True,
        append_images=frames[1:],
        duration=durations,
        loop=loop,
        optimize=False,
        disposal=2,
    )


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: frame.py <gif>")
    frame_gif(sys.argv[1])
