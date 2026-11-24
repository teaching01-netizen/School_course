"""Rebuild the pinned model and enforce an explicit transitive axiom allowlist."""
from pathlib import Path
import hashlib
import json
import re
import subprocess

ROOT = Path(__file__).resolve().parent
EVIDENCE = ROOT / "evidence"
EVIDENCE.mkdir(exist_ok=True)
commands = []


def run(args, output):
    completed = subprocess.run(args, cwd=ROOT, capture_output=True, text=True)
    text = completed.stdout + completed.stderr
    (EVIDENCE / output).write_text(text)
    commands.append({"argv": args, "exit_status": completed.returncode, "output": output})
    (EVIDENCE / "lean-commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    if completed.returncode:
        raise SystemExit(f"FAILED: {args!r}; see evidence/{output}")
    return text


sources = sorted((ROOT / "proof").glob("*.lean"))
names = []
for source in sources:
    text = source.read_text()
    forbidden = re.search(r"\b(sorry|admit|native_decide|unsafe|trace_state)\b|^\s*(axiom|constant)\s+", text, re.M)
    if forbidden:
        raise SystemExit(f"Forbidden proof construct in {source.name}: {forbidden.group(0)}")
    if source.name != "Audit.lean":
        names.extend(re.findall(r"^theorem\s+(\w+)", text, re.M))

run(["lake", "build"], "lean-build.log")
audit = run(["lake", "env", "lean", "proof/Audit.lean"], "theorems-and-axioms.log")
allowed = {"propext", "Quot.sound", "Classical.choice"}
used = set()
for entry in re.findall(r"depends on axioms: \[([^\]]*)\]", audit):
    used.update(item.strip() for item in entry.split(",") if item.strip())
if used - allowed:
    raise SystemExit(f"Unapproved transitive axioms: {used - allowed}")
for name in names:
    if not re.search(r"'AbsenceSubmit\." + re.escape(name) + r"' (depends on axioms:|does not depend on any axioms)", audit):
        raise SystemExit(f"Missing axiom audit for {name}")

lean = run(["lake", "env", "lean", "--version"], "lean-version.log").strip()
lake = run(["lake", "--version"], "lake-version.log").strip()
hashes = {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
          for path in [ROOT / "lean-toolchain", ROOT / "lakefile.lean", ROOT / "lake-manifest.json",
                       ROOT / "AbsenceSubmitOptimization.lean", ROOT / "check.py", *sources]}
result = {"formal_status": "PROVED", "theorem_count": len(names), "lean": lean, "lake": lake,
          "allowed_axioms": sorted(allowed), "used_axioms": sorted(used), "source_sha256": hashes}
(EVIDENCE / "proof-audit.json").write_text(json.dumps(result, indent=2) + "\n")
print(f"PROVED: {len(names)} model theorems; transitive axiom audit passed: {sorted(used)}")
print("Implementation correspondence and production correctness require separate evidence.")
