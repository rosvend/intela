# /// script
# requires-python = ">=3.10"
# dependencies = ["pandas>=2.0", "openpyxl>=3.1"]
# ///
# Diagnostico puntual de los archivos de muestra del cliente.
# Run: uv run --script src/scripts/sample.py
# uv resuelve las dependencias de arriba en un entorno efimero cacheado,
# asi que no se instala nada en el stack del proyecto.

import difflib
import re
import unicodedata
from pathlib import Path

import pandas as pd

FILES_DIR = Path(__file__).resolve().parents[2] / "data" / "files"

MONEY_HINTS = ("monto", "valor", "ingreso", "revenue", "amount", "royalt", "pago", "usd", "cop", "eur")
USAGE_HINTS = ("stream", "duracion", "runtime", "reproduc", "playback", "visualiz")
RIGHTS_HINTS = ("autor", "guionista", "director", "compositor", "editor", "titular", "distributor", "share", "split")
STANDARD_ID_HINTS = ("isrc", "iswc", "eidr", "ipi", "isan", "imdb")
DATE_HINTS = ("fecha", "date", "year", "ano", "periodo", "period")
TITLE_HINTS = ("titulo", "title", "name", "nombre")
PLACEHOLDERS = ("--", "-", "n/a", "na", "null", "none", "sin dato", "")


def normalize_text(value):
    if value is None or (isinstance(value, float) and pd.isna(value)):
        return ""
    text = unicodedata.normalize("NFKD", str(value))
    text = "".join(char for char in text if not unicodedata.combining(char))
    text = re.sub(r"[^a-z0-9]+", " ", text.lower())
    return text.strip()


def columns_matching(columns, hints):
    return [col for col in columns if any(hint in normalize_text(col) for hint in hints)]


def read_sheets(path):
    book = pd.ExcelFile(path)
    return {name: book.parse(name) for name in book.sheet_names}


def profile_columns(frame):
    rows = []
    for col in frame.columns:
        series = frame[col]
        rows.append(
            {
                "column": col,
                "dtype": str(series.dtype),
                "filled_pct": round(100 * series.notna().mean(), 1),
                "unique": int(series.nunique(dropna=True)),
                "sample": next((str(v)[:32] for v in series.dropna()), ""),
            }
        )
    return pd.DataFrame(rows)


def empty_columns(profile):
    return profile.loc[profile["filled_pct"] == 0, "column"].tolist()


def constant_columns(profile):
    return profile.loc[(profile["unique"] == 1) & (profile["filled_pct"] > 0), "column"].tolist()


def key_candidates(frame, profile):
    mask = (profile["unique"] == len(frame)) & (profile["filled_pct"] == 100)
    return profile.loc[mask, "column"].tolist()


def id_columns(frame):
    return [col for col in frame.columns if any(token.startswith("id") for token in normalize_text(col).split())]


def grain_report(frame):
    grain = {}
    for col in id_columns(frame):
        distinct = frame[col].nunique(dropna=True)
        if 0 < distinct < len(frame):
            grain[col] = f"{distinct} distinct / {len(frame)} rows"
    return grain


def placeholder_counts(frame):
    counts = {}
    for col in frame.columns:
        if frame[col].dtype != "object":
            continue
        hits = frame[col].dropna().map(lambda v: normalize_text(v) in PLACEHOLDERS).sum()
        if hits:
            counts[col] = int(hits)
    return counts


def date_coverage(frame):
    coverage = {}
    for col in columns_matching(frame.columns, DATE_HINTS):
        values = frame[col].dropna()
        if values.empty:
            continue
        coverage[col] = (str(values.min())[:24], str(values.max())[:24])
    return coverage


def normalized_titles(frame, columns):
    titles = set()
    for col in columns:
        titles.update(normalize_text(v) for v in frame[col].dropna())
    titles.discard("")
    return titles


def describe_sheet(label, frame):
    profile = profile_columns(frame)
    print(f"\n### {label}")
    print(f"rows: {len(frame)} | columns: {len(frame.columns)}")
    print(f"duplicated full rows: {int(frame.duplicated().sum())}")
    print(f"columns fully empty: {empty_columns(profile) or 'none'}")
    print(f"columns with a single value: {constant_columns(profile) or 'none'}")
    print(f"unique key candidates: {key_candidates(frame, profile) or 'none'}")
    print(f"money columns: {columns_matching(frame.columns, MONEY_HINTS) or 'NONE'}")
    print(f"usage columns: {columns_matching(frame.columns, USAGE_HINTS) or 'none'}")
    print(f"rights columns: {columns_matching(frame.columns, RIGHTS_HINTS) or 'none'}")
    print(f"external ids: {columns_matching(frame.columns, STANDARD_ID_HINTS) or 'none'}")
    print(f"date coverage: {date_coverage(frame) or 'none'}")
    print(f"placeholder values: {placeholder_counts(frame) or 'none'}")
    print(f"repeated ids (grain): {grain_report(frame) or 'none'}")
    print("\ncolumn profile:")
    print(profile.to_string(index=False))


def describe_file(path):
    print("\n" + "=" * 78)
    print(f"FILE: {path.name}")
    sheets = read_sheets(path)
    print(f"sheets: {list(sheets)}")
    for name, frame in sheets.items():
        describe_sheet(f"{path.name} :: {name}", frame)
    return sheets


def compare_columns(frame_a, frame_b):
    set_a = {normalize_text(c) for c in frame_a.columns}
    set_b = {normalize_text(c) for c in frame_b.columns}
    print(f"\nshared column names: {sorted(set_a & set_b) or 'none'}")


def compare_titles(name_a, frame_a, name_b, frame_b):
    cols_a = columns_matching(frame_a.columns, TITLE_HINTS)
    cols_b = columns_matching(frame_b.columns, TITLE_HINTS)
    titles_a = normalized_titles(frame_a, cols_a)
    titles_b = normalized_titles(frame_b, cols_b)
    shared = sorted(titles_a & titles_b)
    print(f"\n{name_a} title columns: {cols_a}")
    print(f"{name_b} title columns: {cols_b}")
    print(f"distinct normalized titles: {name_a}={len(titles_a)} {name_b}={len(titles_b)}")
    print(f"exact normalized matches: {len(shared)}")
    print(f"examples: {shared[:15] or 'none'}")
    return titles_a, titles_b


def fuzzy_title_preview(titles_a, titles_b, cutoff=0.6, limit=10):
    pairs = []
    pool = sorted(titles_b)
    for title in sorted(titles_a):
        best = difflib.get_close_matches(title, pool, n=1, cutoff=cutoff)
        if best:
            score = difflib.SequenceMatcher(None, title, best[0]).ratio()
            pairs.append((round(score, 2), title, best[0]))
    pairs.sort(reverse=True)
    print(f"\nfuzzy candidates above {cutoff}: {len(pairs)}")
    for score, left, right in pairs[:limit]:
        print(f"  {score}  {left}  ->  {right}")


def value_lookup(frame, column, needle):
    if column not in frame.columns:
        return
    values = frame[column].dropna().map(normalize_text)
    hits = values[values.str.contains(needle, na=False)]
    print(f"\nrows where {column} contains '{needle}': {len(hits)} of {len(frame)}")


def main():
    pd.set_option("display.width", 200)
    pd.set_option("display.max_rows", 200)

    paths = sorted(FILES_DIR.glob("*.xls*"))
    loaded = [describe_file(path) for path in paths]
    frames = [(sheet, frame) for sheets in loaded for sheet, frame in sheets.items()]

    print("\n" + "=" * 78)
    print("CROSS FILE CHECKS")

    for i in range(len(frames)):
        for j in range(i + 1, len(frames)):
            name_a, frame_a = frames[i]
            name_b, frame_b = frames[j]
            print("\n" + "=" * 60)
            print(f"{name_a}  vs  {name_b}")
            compare_columns(frame_a, frame_b)
            titles_a, titles_b = compare_titles(name_a, frame_a, name_b, frame_b)
            fuzzy_title_preview(titles_a, titles_b)

    for _, frame in frames:
        value_lookup(frame, "distributor", "caracol")


if __name__ == "__main__":
    main()
