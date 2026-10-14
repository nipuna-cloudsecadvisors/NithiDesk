"""Precision/recall of each scorer over the synthetic pairs. Run: python -I -X utf8 evaluate.py [--detail]"""
import sys

from match import SCORERS, _key, score_structured
from pairs import PAIRS
from sinhala import normalise

THRESHOLDS = [0.3, 0.4, 0.5, 0.6, 0.7]


def prf(scorer, t, rows=PAIRS):
    tp = fp = fn = 0
    for _, a, b, y in rows:
        p = scorer(a, b) >= t
        tp += p and y; fp += p and not y; fn += (not p) and y
    prec = tp / (tp + fp) if tp + fp else 1.0
    rec = tp / (tp + fn) if tp + fn else 1.0
    return prec, rec


def main():
    n_pos = sum(y for *_, y in PAIRS)
    print(f"{len(PAIRS)} pairs ({n_pos} match, {len(PAIRS) - n_pos} non-match)\n")
    print("| Scorer | " + " | ".join(f"t={t}" for t in THRESHOLDS) + " |")
    print("|---|" + "---|" * len(THRESHOLDS))
    for name, f in SCORERS.items():
        cells = [f"P {p:.2f} / R {r:.2f}" for p, r in (prf(f, t) for t in THRESHOLDS)]
        print(f"| {name} | " + " | ".join(cells) + " |")

    print("\nRecall by category, V5 structured at t=0.6 (non-match row shows specificity):")
    cats = sorted({c for c, *_ in PAIRS})
    for c in cats:
        rows = [r for r in PAIRS if r[0] == c]
        ok = sum((score_structured(a, b) >= 0.6) == y for _, a, b, y in rows)
        print(f"- {c}: {ok}/{len(rows)}")

    typing = [r for r in PAIRS if r[0] == "typing"]
    same = sum(normalise(a, False) == normalise(b, False) for _, a, b, _ in typing)
    print(f"\nTyping variants identical after normalise(): {same}/{len(typing)}")

    if "--detail" in sys.argv:
        print("\nScores (V5 | V3 key):")
        for c, a, b, y in PAIRS:
            s = score_structured(a, b)
            flag = "" if (s >= 0.6) == y else "  <-- WRONG at 0.6"
            print(f"{c:12} {y!s:5} {s:.2f}  {_key(a)!r} vs {_key(b)!r}{flag}")


if __name__ == "__main__":
    main()
