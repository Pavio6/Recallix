#!/usr/bin/env python3
"""
Normalize simple reference records into numbered GB/T 7714-like entries.

Input via stdin:
[
  {
    "type": "book",
    "authors": ["张三"],
    "title": "大模型系统设计",
    "place": "北京",
    "publisher": "电子工业出版社",
    "year": "2025"
  }
]
"""

import json
import sys


def join_authors(authors):
    return ", ".join(a.strip() for a in authors if str(a).strip()) or "作者未提供"


def normalize(record, index):
    ref_type = str(record.get("type", "")).strip().lower()
    authors = join_authors(record.get("authors", []))
    title = str(record.get("title", "")).strip() or "题名未提供"
    year = str(record.get("year", "")).strip() or "年份未提供"

    if ref_type == "book":
        place = str(record.get("place", "")).strip() or "出版地未提供"
        publisher = str(record.get("publisher", "")).strip() or "出版者未提供"
        body = f"{authors}. {title}[M]. {place}: {publisher}, {year}."
    elif ref_type == "journal":
        journal = str(record.get("journal", "")).strip() or "刊名未提供"
        volume = str(record.get("volume", "")).strip()
        issue = str(record.get("issue", "")).strip()
        pages = str(record.get("pages", "")).strip()
        volume_issue = volume
        if issue:
            volume_issue = f"{volume}({issue})" if volume else f"({issue})"
        suffix = f", {volume_issue}" if volume_issue else ""
        if pages:
            suffix += f": {pages}"
        body = f"{authors}. {title}[J]. {journal}, {year}{suffix}."
    elif ref_type == "thesis":
        place = str(record.get("place", "")).strip() or "城市未提供"
        school = str(record.get("school", "")).strip() or "学校未提供"
        body = f"{authors}. {title}[D]. {place}: {school}, {year}."
    else:
        body = f"{authors}. {title}. 文献类型未提供, {year}."

    return f"[{index}] {body}"


def main():
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        print(f"invalid json: {exc}", file=sys.stderr)
        return 1

    if not isinstance(payload, list):
        print("input must be a JSON array", file=sys.stderr)
        return 1

    for index, record in enumerate(payload, start=1):
        if not isinstance(record, dict):
            print(f"[{index}] invalid record")
            continue
        print(normalize(record, index))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
