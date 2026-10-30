#!/usr/bin/env python3
"""Check the locale parity of the user documentation set (docs/user/).

The English source docs/user/README.md is the reference. Every locale file
docs/user/README.<locale>.md must mirror it section-for-section:

1. The ordered sequence of numbered section keys ("## N.") must be identical
   to the English source's sequence.
2. Every section must carry at least one non-empty content line beyond the
   heading (no empty stubs).
3. The introduction (everything before the first section heading) must carry
   at least one non-empty content line beyond the H1 title.
4. No file may contain untranslated-work markers (TODO/TBD/FIXME).

Exit code 0 means every locale passes; non-zero means at least one failure
(the failing details are printed). The accepted locale set is the owner's
locked list: en, zh-CN, es, pt-BR, ja, ru, ko, de, fr, id, tr, vi.
"""

import re
import sys
from pathlib import Path

DOCS = Path(__file__).resolve().parent.parent.parent / "docs" / "user"
EN_FILE = "README.md"
LOCALES = ["en", "zh-CN", "es", "pt-BR", "ja", "ru", "ko", "de", "fr", "id", "tr", "vi"]
HEADING = re.compile(r"^##\s+(\d+)\.\s")
MARKERS = re.compile(r"\b(TODO|TBD|FIXME)\b")


def locale_file(locale: str) -> Path:
    if locale == "en":
        return DOCS / EN_FILE
    return DOCS / f"README.{locale}.md"


def parse(path: Path):
    """Return (keys_in_order, {key: body_line_count}, intro_content_lines)."""
    keys, counts = [], {}
    intro = 0
    current = None
    seen_heading = False
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        m = HEADING.match(raw)
        if m:
            seen_heading = True
            key = m.group(1)
            if key in counts:
                raise ValueError(f"{path.name}: duplicate section key {key}")
            keys.append(key)
            counts[key] = 0
            current = key
            continue
        if not line:
            continue
        if current is not None:
            counts[current] += 1
        elif not seen_heading and not line.startswith("# "):
            intro += 1
    return keys, counts, intro


def main() -> int:
    failures = []
    reference_path = locale_file("en")
    if not reference_path.exists():
        print(f"FAIL: the English source {reference_path} does not exist")
        return 1
    ref_keys, ref_counts, ref_intro = parse(reference_path)
    if not ref_keys:
        print("FAIL: the English source carries no numbered section keys")
        return 1
    print(f"reference: {reference_path.name} — {len(ref_keys)} sections "
          f"({', '.join(ref_keys)}), intro content lines: {ref_intro}")

    for locale in LOCALES:
        path = locale_file(locale)
        if not path.exists():
            failures.append(f"{locale}: missing file {path}")
            continue
        text = path.read_text(encoding="utf-8")
        m = MARKERS.search(text)
        if m:
            failures.append(f"{locale}: untranslated-work marker {m.group(1)} found")
        try:
            keys, counts, intro = parse(path)
        except ValueError as exc:
            failures.append(f"{locale}: {exc}")
            continue
        if keys != ref_keys:
            missing = [k for k in ref_keys if k not in keys]
            extra = [k for k in keys if k not in ref_keys]
            order_bad = [k for k in keys if k in ref_keys and k not in missing] != \
                        [k for k in ref_keys if k not in extra]
            failures.append(
                f"{locale}: section keys diverge (missing={missing}, extra={extra}, "
                f"order matches={not order_bad})"
            )
            continue
        empty = [k for k, n in counts.items() if n == 0]
        if empty:
            failures.append(f"{locale}: empty sections {empty}")
        if intro < 1:
            failures.append(f"{locale}: the introduction carries no content beyond the title")
        if not failures or not any(f.startswith(f"{locale}:") for f in failures):
            print(f"OK: {path.name} — {len(keys)} sections mirror the source, "
                  f"intro content lines: {intro}")

    if failures:
        print("\nPARITY FAILURES:")
        for f in failures:
            print(f"  - {f}")
        return 1
    print(f"\nparity OK: {len(LOCALES)}/{len(LOCALES)} locale files present and "
          f"mirroring the English source ({len(LOCALES) - 1} translations)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
