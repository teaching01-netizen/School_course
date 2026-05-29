# Anti-Forgetting Rules

Before modifying a module, identify what old capabilities must not break.

Checklist:

1. What user workflow already depends on this?
2. What invariant must remain true?
3. What regression protection exists (test/fixture/replay)?
4. If none exists and the behavior is critical, add characterization/regression protection first.
5. Does the change require backward-compatibility or data migration?

