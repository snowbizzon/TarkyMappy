#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
TarkyMappy — обновление цен барахолки.

ЧТО ЭТО ДЕЛАЕТ
==============
Скачивает СРЕДНИЕ ЦЕНЫ ЗА 24 ЧАСА с игровой барахолки Escape from
Tarkov (источник: открытый API сообщества tarkov.dev). Данные по обоим
режимам игры:

    regular — обычная PvP-барахолка
    pve     — барахолка режима PvE (отдельная экономика)

Для КАЖДОГО режима считается ЦЕНА ЗА 1 СЛОТ инвентаря:

    pp = цена / (ширина x высота)

и берётся ТОП самых дорогих за слот предметов (по умолчанию 50).
Результат пишется в:

    prices.json  — РЯДОМ С ЭТИМ СКРИПТОМ

Поэтому держи этот скрипт в папке prices/ рядом с TarkyMappy.exe — тогда
свежие цены всегда попадают куда нужно. Приложение читает файл при КАЖДОМ
открытии панели F2: пересобирать exe НЕ нужно, достаточно закрыть и снова
открыть F2 (или перезапустить приложение).

ОТКУДА ЦЕНЫ (два источника, 2026-10)
====================================
1) GraphQL  https://api.tarkov.dev/graphql   — основной, лёгкий запрос.
2) СТАТИКА  https://json.tarkov.dev/<mode>/items — запасной. С осени 2026
   GraphQL периодически лежит (HTTP 422 "GraphQL server unavailable"),
   а статические снапшоты обновляются каждый день. В статике поля те же
   (avg24hPrice/width/height), но имена — заглушки "<id> ShortName",
   поэтому shortName подтягивается из json.tarkov.dev/<mode>/items_en
   (flat {"<id> ShortName"/"<id> Name": "текст"}) по id предмета.

Большие файлы (~17МБ) качаются ПО КУСКАМ через HTTP Range: сервер
периодически рвёт соединение целиком, а докачка кусков проходит надёжно.

КАК ЗАПУСКАТЬ
=============
Нужен Python 3.8+. Внешних библиотек НЕТ — только стандартная библиотека.
Из папки со скриптом:

    python update_prices.py

Необязательные аргументы:

    --top 100              # размер топа (по умолчанию 50)
    --out путь             # куда писать prices.json (по умолчанию — рядом со скриптом)
    --source graphql|static|auto   # источник (по умолчанию auto: GraphQL, при 422 — статика)

ФОРМАТ prices.json
==================
{
  "updated": "2026-09-05T12:34:56+00:00",  # когда скачаны цены (UTC)
  "modes": {
    "regular": [ {"n": имя, "w": ширина, "h": высота,
                  "p": цена за шт, "pp": цена за слот}, ... ],
    "pve":     [ ...то же самое для PvE... ]
  }
}

Оба списка отсортированы по pp (цена за слот) по убыванию.
"""

import argparse
import json
import urllib.request
import urllib.error
from datetime import datetime, timezone
from pathlib import Path

# --- Настройки источника данных -------------------------------------------

GRAPHQL_URL = "https://api.tarkov.dev/graphql"
STATIC_ITEMS = "https://json.tarkov.dev/{mode}/items"       # цены + размеры
STATIC_NAMES = "https://json.tarkov.dev/regular/items_en"   # {"<id> ShortName"/"<id> Name": текст}
DEFAULT_TOP = 50                             # сколько предметов держать в топе
RETRIES = 3                                  # попыток на один запрос/кусок
TIMEOUT = 60                                 # секунд на один HTTP-запрос
CHUNK = 2 * 1024 * 1024                      # кусок Range-скачивания (2МБ)

# Режимы игры. Если tarkov.dev добавит третий — просто допиши значение
# сюда, остальное подхватится само.
MODES = ("regular", "pve")

# GraphQL-запрос. %s подставит имя режима (regular / pve).
QUERY = "{ items(lang: en, gameMode: %s) { shortName width height avg24hPrice } }"

# По умолчанию пишем РЯДОМ СО СКРИПТОМ: скрипт живёт в prices/ рядом с exe,
# значит файл всегда оказывается там, где его ждёт приложение.
# (--out переопределяет.)
OUT_FILE = Path(__file__).resolve().parent / "prices.json"


# --- HTTP-утилиты -----------------------------------------------------------

UA = "TarkyMappy-update-prices/1.0"


def _http(request: urllib.request.Request):
    """Один запрос; возвращает bytes. Ошибки отдаёт наверх."""
    request.add_header("User-Agent", UA)
    with urllib.request.urlopen(request, timeout=TIMEOUT) as resp:
        return resp.read()


def _content_length(url: str) -> int:
    """Размер файла: HEAD, а при 403/ошибке — GET с Range 0-0 (Content-Range)."""
    last_err = None
    for attempt in range(1, RETRIES + 1):
        try:
            req = urllib.request.Request(url, method="HEAD")
            with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
                return int(resp.headers.get("Content-Length", "0"))
        except Exception as err:
            last_err = err
            try:
                rq = urllib.request.Request(url, headers={"Range": "bytes=0-0", "User-Agent": UA})
                with urllib.request.urlopen(rq, timeout=TIMEOUT) as resp:
                    cr = resp.headers.get("Content-Range", "")  # "bytes 0-0/17308744"
                    total = cr.rsplit("/", 1)[-1]
                    if total.isdigit():
                        return int(total)
            except Exception as err2:
                last_err = err2
    raise RuntimeError("нет Content-Length для %s (%s)" % (url, last_err))


def _post_graphql(query: str) -> dict:
    """GraphQL-запрос -> распарсенный JSON. 422/ошибки сети — исключение."""
    body = json.dumps({"query": query}).encode("utf-8")
    last_err = None
    for attempt in range(1, RETRIES + 1):
        try:
            req = urllib.request.Request(
                GRAPHQL_URL,
                data=body,
                headers={"Content-Type": "application/json"},
            )
            return json.loads(_http(req).decode("utf-8"))
        except Exception as err:
            last_err = err
            print("  graphql: попытка %d/%d не удалась: %s" % (attempt, RETRIES, err))
    raise last_err


def _download_chunked(url: str) -> bytes:
    """Скачивает url ЦЕЛИКОМ по Range-кускам (сервер рвёт длинные соединения).

    Размер узнаём через HEAD/Range-запрос; каждый кусок качаем
    со своими ретраями, недокачанный кусок — заново.
    """
    total = _content_length(url)
    if total <= 0:
        raise RuntimeError("пустой файл: %s" % url)

    parts = []
    done = 0
    while done < total:
        end = min(done + CHUNK - 1, total - 1)
        buf = None
        for attempt in range(1, RETRIES + 1):
            try:
                rq = urllib.request.Request(url, headers={"Range": "bytes=%d-%d" % (done, end)})
                buf = _http(rq)
                if len(buf) == end - done + 1:
                    break
                buf = None  # недокачан — повтор
            except Exception as err:
                if attempt == RETRIES:
                    raise RuntimeError("кусок %d-%d %s: %s" % (done, end, url, err))
        if buf is None:
            raise RuntimeError("кусок %d-%d %s: не скачался" % (done, end, url))
        parts.append(buf)
        done += len(buf)
        print("    ...%d/%d байт" % (done, total))
    return b"".join(parts)


def _load_static_json(url: str) -> dict:
    return json.loads(_download_chunked(url).decode("utf-8"))


# --- Источник 1: GraphQL ----------------------------------------------------

def fetch_graphql(mode: str) -> list:
    """Топ-сырьё одного режима из GraphQL: [{n,w,h,p}, ...]."""
    data = _post_graphql(QUERY % mode)
    if data.get("errors"):
        raise RuntimeError("; ".join(e.get("message", "?") for e in data["errors"]))
    items = data["data"]["items"]
    return [
        {"n": it["shortName"], "w": int(it["width"]), "h": int(it["height"]),
         "p": int(round(it["avg24hPrice"]))}
        for it in items
        if it["avg24hPrice"] and it["avg24hPrice"] > 0
        and it["width"] and it["width"] > 0
        and it["height"] and it["height"] > 0
        and it["shortName"]
    ]


# --- Источник 2: статические снапшоты json.tarkov.dev -----------------------

def _load_name_dict():
    """{"<id> ShortName"/"<id> Name": текст} -> (short: {id: текст}, name: {id: текст})."""
    raw = _load_static_json(STATIC_NAMES)
    data = raw.get("data", raw)
    short, name = {}, {}
    import re
    for key, val in data.items():
        m = re.match(r"^([0-9a-f]{24}) (ShortName|Name)$", key)
        if not m or not val:
            continue
        (short if m.group(2) == "ShortName" else name)[m.group(1)] = val
    return short, name


def fetch_static(mode: str) -> list:
    """Топ-сырьё одного режима из статического снапшота."""
    data = _load_static_json(STATIC_ITEMS.format(mode=mode))["data"]["items"]
    short, name = _load_name_dict()
    out = []
    for iid, it in data.items():
        price = it.get("avg24hPrice") or 0
        w = it.get("width") or 0
        h = it.get("height") or 0
        if price <= 0 or w <= 0 or h <= 0:
            continue
        n = short.get(iid) or name.get(iid)
        if not n:
            continue  # без имени предмет в топе бесполезен
        out.append({"n": n, "w": int(w), "h": int(h), "p": int(round(price))})
    return out


# --- Общее ------------------------------------------------------------------

def add_price_per_slot(items):
    """Считает pp (цену за 1 слот) и сортирует список по ней по убыванию."""
    for it in items:
        it["pp"] = int(round(it["p"] / (it["w"] * it["h"])))
    items.sort(key=lambda it: (-it["pp"], -it["p"], it["n"]))
    return items


def fetch_mode(mode: str, source: str) -> list:
    """Один режим; auto = GraphQL, при неудаче — статика (с причиной в лог)."""
    if source in ("graphql", "auto"):
        try:
            return fetch_graphql(mode)
        except Exception as err:
            if source == "graphql":
                raise
            print("  GraphQL недоступен (%s) — перехожу на статический снапшот" % err)
    return fetch_static(mode)


# --- Точка входа ------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Обновить цены барахолки для TarkyMappy")
    parser.add_argument("--top", type=int, default=DEFAULT_TOP,
                        help="сколько предметов держать в топе (по умолчанию %d)" % DEFAULT_TOP)
    parser.add_argument("--out", type=str, default=str(OUT_FILE),
                        help="куда писать prices.json (по умолчанию — рядом со скриптом)")
    parser.add_argument("--source", choices=("auto", "graphql", "static"), default="auto",
                        help="источник цен: auto (GraphQL, иначе статика), graphql, static")
    args = parser.parse_args()

    modes_out = {}
    for mode in MODES:
        print("- режим %s: скачиваю ..." % mode)
        items = add_price_per_slot(fetch_mode(mode, args.source))[: args.top]
        modes_out[mode] = items
        if items:
            print("  режим %s: %d предметов, топ-1 = %s (%d за слот)"
                  % (mode, len(items), items[0]["n"], items[0]["pp"]))
        else:
            print("  режим %s: пусто" % mode)

    out = {
        "updated": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "modes": modes_out,
    }
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    # UTF-8 без BOM — так файл читает приложение.
    out_path.write_text(json.dumps(out, ensure_ascii=False, indent=1), encoding="utf-8")
    print("OK: %s (данные от %s)" % (out_path, out["updated"]))
    print("Пересборка не нужна: открой панель F2 в TarkyMappy заново.")


if __name__ == "__main__":
    main()
