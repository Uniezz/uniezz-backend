"""Posts the number of onboarded students and a growth chart to Telegram.

Reads DATABASE_URL, TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID and TELEGRAM_THREAD_ID
(name:id pairs; the chart goes to the "newcomers" topic). With --out it only
writes the chart to that file and prints the caption, for trying it locally.
"""

import argparse
import datetime as dt
import io
import json
import os
import sys
import urllib.error
import urllib.request
import uuid
from zoneinfo import ZoneInfo

import matplotlib

matplotlib.use("Agg")

import matplotlib.dates as mdates
import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import psycopg

TIMEZONE = ZoneInfo("Europe/Warsaw")
THREAD_NAME = "newcomers"
TELEGRAM_API = "https://api.telegram.org"
SEND_TIMEOUT_SECONDS = 30

SURFACE = "#fcfcfb"
TEXT_PRIMARY = "#0b0b0b"
TEXT_SECONDARY = "#52514e"
GRID = "#e4e3df"
SERIES = "#2a78d6"

ONBOARDED_PER_DAY = """
    SELECT (onboarded_at AT TIME ZONE 'Europe/Warsaw')::date AS day, COUNT(*)
    FROM users
    WHERE onboarded_at IS NOT NULL AND deleted_at IS NULL AND banned_at IS NULL
    GROUP BY day
    ORDER BY day
"""


def load_per_day(database_url):
    with psycopg.connect(database_url, connect_timeout=15) as conn:
        conn.read_only = True
        with conn.cursor() as cur:
            cur.execute(ONBOARDED_PER_DAY)
            return {day: count for day, count in cur.fetchall()}


def daily_series(per_day, today):
    start = min(per_day, default=today)
    days, daily, totals = [], [], []
    total = 0
    day = start
    while day <= today:
        n = per_day.get(day, 0)
        total += n
        days.append(day)
        daily.append(n)
        totals.append(total)
        day += dt.timedelta(days=1)
    return days, daily, totals


def caption(today, daily, totals):
    return f"📊 {today.day} {today:%b}: {totals[-1]} students (+{daily[-1]} today)"


def draw(days, daily, totals):
    fig, (top, bottom) = plt.subplots(
        2, 1, sharex=True, figsize=(10, 6), dpi=150,
        gridspec_kw={"height_ratios": [3, 1.3], "hspace": 0.25},
    )
    fig.patch.set_facecolor(SURFACE)

    top.plot(days, totals, color=SERIES, linewidth=2, solid_capstyle="round")
    top.scatter([days[-1]], [totals[-1]], s=40, color=SERIES, zorder=3,
                edgecolors=SURFACE, linewidths=2)
    top.annotate(f"{totals[-1]}", (days[-1], totals[-1]), xytext=(8, 0),
                 textcoords="offset points", va="center", fontsize=11,
                 fontweight="bold", color=TEXT_PRIMARY)
    top.set_title("Students who finished onboarding", loc="left", fontsize=13,
                  color=TEXT_PRIMARY, pad=12)
    top.set_ylim(bottom=0)

    bottom.bar(days, daily, width=0.8, color=SERIES)
    bottom.set_title("New per day", loc="left", fontsize=10, color=TEXT_SECONDARY)
    bottom.set_ylim(bottom=0)

    for ax in (top, bottom):
        ax.set_facecolor(SURFACE)
        ax.grid(axis="y", color=GRID, linewidth=0.8)
        ax.set_axisbelow(True)
        for side in ("top", "right", "left"):
            ax.spines[side].set_visible(False)
        ax.spines["bottom"].set_color(GRID)
        ax.tick_params(colors=TEXT_SECONDARY, labelsize=9, length=0)
        ax.yaxis.set_major_locator(mticker.MaxNLocator(integer=True, nbins=5))

    locator = mdates.AutoDateLocator(minticks=3, maxticks=8)
    bottom.xaxis.set_major_locator(locator)
    bottom.xaxis.set_major_formatter(mdates.ConciseDateFormatter(locator))
    top.margins(x=0.04)

    buf = io.BytesIO()
    fig.savefig(buf, format="png", facecolor=SURFACE, bbox_inches="tight")
    plt.close(fig)
    return buf.getvalue()


def thread_id(value, name):
    for pair in value.split(","):
        key, sep, raw = pair.partition(":")
        if sep and key.strip() == name and raw.strip().isdigit():
            return int(raw.strip())
    return None


def send_photo(token, chat_id, thread, png, text):
    boundary = uuid.uuid4().hex
    fields = {
        "chat_id": chat_id,
        "message_thread_id": str(thread),
        "caption": text,
    }
    body = io.BytesIO()
    for name, value in fields.items():
        body.write(f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n".encode())
        body.write(value.encode() + b"\r\n")
    body.write(
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"photo\"; filename=\"students.png\"\r\n"
        "Content-Type: image/png\r\n\r\n".encode()
    )
    body.write(png + b"\r\n")
    body.write(f"--{boundary}--\r\n".encode())

    request = urllib.request.Request(
        f"{TELEGRAM_API}/bot{token}/sendPhoto",
        data=body.getvalue(),
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=SEND_TIMEOUT_SECONDS) as response:
            answer = json.load(response)
    except urllib.error.HTTPError as err:
        sys.exit(f"telegram answered {err.code}: {err.read().decode(errors='replace')[:300]}")
    except urllib.error.URLError as err:
        sys.exit(f"send photo: {err.reason}")
    if not answer.get("ok"):
        sys.exit(f"telegram refused the photo: {answer.get('description')}")


def require(name):
    value = os.environ.get(name, "").strip()
    if not value:
        sys.exit(f"{name} is not set")
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--out", help="write the chart here and send nothing")
    args = parser.parse_args()

    today = dt.datetime.now(TIMEZONE).date()
    days, daily, totals = daily_series(load_per_day(require("DATABASE_URL")), today)
    png = draw(days, daily, totals)
    text = caption(today, daily, totals)

    if args.out:
        with open(args.out, "wb") as f:
            f.write(png)
        print(text)
        return

    thread = thread_id(require("TELEGRAM_THREAD_ID"), THREAD_NAME)
    if thread is None:
        sys.exit(f"TELEGRAM_THREAD_ID has no {THREAD_NAME}:<id> pair")
    send_photo(require("TELEGRAM_BOT_TOKEN"), require("TELEGRAM_CHAT_ID"), thread, png, text)
    print(text)


if __name__ == "__main__":
    main()
