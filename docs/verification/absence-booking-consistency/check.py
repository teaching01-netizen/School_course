"""Recheck this verification package. Go probes require a dedicated local TEST_DATABASE_URL."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]
BACKEND = REPO / "backend"
API = BACKEND / "internal/httpapi/absenceshttp"

MUTANTS = [
    ("Policy", "if Precedes a b then a else b", "if a.rank ≤ b.rank then a else b", "choose_minimum"),
    ("Policy", "| .admin => courses", "| .admin => courses.filter Course.bookable", "visibility_exact"),
    ("Policy", "hasRoot && !priorities.isEmpty", "hasRoot && priorities.isEmpty", "inherited_rule_keeps_priorities"),
    ("Policy", "instant < cal.midnight (through + 1)", "instant ≤ cal.midnight (through + 1)", "local_dates_iff_half_open"),
    ("Batch", "some (r.scope, group, r.sessions.mergeSort)", "some (r.scope, group, [])", "shares_only_same_scope_group_and_sessions"),
    ("Traces", "then some p else s.journal k", "then some { p with batch := { p.batch with rows := p.batch.rows.take 1 } } else s.journal k", "new_commit_publishes_exact_batch"),
]


def run(command, cwd=HERE):
    return subprocess.run(command, cwd=cwd, text=True, capture_output=True, check=False)


def output(result):
    return result.stdout + result.stderr


def replace_one(source, old, new):
    if source.count(old) != 1:
        raise ValueError(f"Mutation anchor is missing or ambiguous: {old}")
    return source.replace(old, new)


def lean_checks():
    result = run(["lake", "build"])
    log = output(result)
    print(log, end="")
    result.check_returncode()
    names = set()
    for file in (HERE / "proof").glob("*.lean"):
        source = file.read_text()
        if re.search(r"\b(sorry|admit|axiom|native_decide|unsafe)\b", source):
            raise ValueError(f"Trust escape or placeholder in {file}")
        names.update(re.findall(r"(?m)^#print axioms (\w+)$", source))
    audited = re.findall(r"'Booking\.(\w+)' (depends on axioms: \[([^\]]*)\]|does not depend on any axioms)", log)
    if {name for name, _, _ in audited} != names:
        raise ValueError("Build output is missing an axiom result")
    allowed = {"propext", "Classical.choice", "Quot.sound"}
    for name, _, axioms in audited:
        used = set(axioms.split(", ")) if axioms else set()
        if used - allowed:
            raise ValueError(f"{name} uses unexpected axioms: {used - allowed}")
    print(f"Audited {len(names)} theorem statements")


def lean_mutants():
    with tempfile.TemporaryDirectory(prefix="booking-lean-mutants-") as tmp:
        for module, old, new, theorem in MUTANTS:
            source = (HERE / "proof" / f"{module}.lean").read_text()
            mutated = replace_one(source, old, new)
            lines = mutated.splitlines()
            first = next(i for i, line in enumerate(lines, 1) if line.startswith(f"theorem {theorem} "))
            end = next(i for i, line in enumerate(lines[first:], first + 1)
                       if line.startswith(("theorem ", "def ", "inductive ", "structure ", "#print ")))
            path = Path(tmp) / f"{module}.lean"
            path.write_text(mutated)
            result = run(["lake", "env", "lean", str(path)])
            failures = re.findall(r"(?m)^.*?:(\d+):\d+: error: (.*)$", output(result))
            hits = [(line, message) for line, message in failures if first <= int(line) < end]
            if result.returncode != 1 or not hits:
                raise ValueError(f"Mutant failed outside {theorem} or survived:\n{output(result)}")
            line, message = hits[0]
            print(f"KILLED {module}.{theorem} at mutant line {line}: {message}")


def go_overlay(virtual, replacement, test):
    with tempfile.TemporaryDirectory(prefix="booking-go-overlay-") as tmp:
        path = Path(tmp)
        source = path / virtual.name
        source.write_text(replacement)
        overlay = path / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(virtual): str(source)}}))
        result = run(["go", "test", "-v", f"-overlay={overlay}", "./internal/httpapi/absenceshttp",
                      "-run", f"^{test}$", "-count=1"], BACKEND)
        print(output(result), end="")
        return result


def go_probes():
    from urllib.parse import urlsplit

    db = os.environ.get("TEST_DATABASE_URL", "")
    if urlsplit(db).hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("Use a dedicated local PostgreSQL database for the probe")
    probe = HERE / "evidence" / "policy-publication-probe.go.txt"
    result = go_overlay(API / "booking_publication_probe_test.go", probe.read_text(),
                        "TestBookingProbePolicyChangeBeforeCommit")
    if result.returncode != 0 or "COUNTEREXAMPLE: policy disable committed" not in output(result):
        raise ValueError("Policy publication counterexample did not reproduce")
    path = API / "sitin_validation.go"
    original = path.read_text()
    mutant = replace_one(original, "\tstarted := time.Now()",
                         "\tfrom = dateFrom.Time\n\ttoExclusive = dateTo.Time.AddDate(0, 0, 2)\n\tstarted := time.Now()")
    result = go_overlay(path, mutant, "TestSatVerbalSubmitUsesInstituteDateForEarlyMissedClass")
    if (result.returncode != 1 or "status = 400, want 201" not in output(result)
            or '"code":"invalid_sessions"' not in output(result)):
        raise ValueError("Timezone mutant failed for the wrong reason or survived")
    print("KILLED Go UTC/two-day selection window: public submit returned invalid_sessions")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--go-probes", action="store_true")
    args = parser.parse_args()
    lean_checks()
    lean_mutants()
    if args.go_probes:
        go_probes()
