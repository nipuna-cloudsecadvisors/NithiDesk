"""Docs checks for CI: relative links resolve, Mermaid blocks are closed and non-empty,
and every planning marker is spelled consistently. Usage: python check_docs.py [root]"""
import pathlib, re, sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()
SKIP = {".git", "node_modules", ".claude"}
LINK = re.compile(r"\[[^\]]*\]\(([^)\s]+)\)")
MARKER = re.compile(r"\[(NEEDS LAWYER CONFIRMATION|MAKE CONFIGURABLE|ASSUMPTION|DECISION REQUIRED)\]", re.I)
errors = []

for md in sorted(root.rglob("*.md")):
    if SKIP & set(md.relative_to(root).parts):
        continue
    rel = md.relative_to(root).as_posix()
    text = md.read_text(encoding="utf-8")
    # Ignore links inside fenced code blocks.
    prose = re.sub(r"```.*?```", "", text, flags=re.S)
    for target in LINK.findall(prose):
        if re.match(r"^[a-z]+:", target) or target.startswith("#"):
            continue
        path = target.split("#", 1)[0]
        if path and not (md.parent / path).exists():
            errors.append(f"{rel}: broken link -> {target}")
    fences = re.findall(r"^```", text, flags=re.M)
    if len(fences) % 2:
        errors.append(f"{rel}: unclosed code fence")
    for block in re.findall(r"```mermaid\n(.*?)```", text, flags=re.S):
        if not block.strip():
            errors.append(f"{rel}: empty mermaid block")
    for m in MARKER.finditer(text):
        if m.group(1) != m.group(1).upper():
            errors.append(f"{rel}: marker should be upper case: {m.group(0)}")

for e in errors:
    print(f"::error::{e}")
print(f"checked docs under {root}: {len(errors)} problem(s)")
sys.exit(1 if errors else 0)
