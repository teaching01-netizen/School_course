import type {
  LeavePolicyCourseRule,
  LeavePolicyTestInput,
  LeavePolicyTestResult,
  MakeupOption,
} from "../../types";

/**
 * Hard-coded SAT Verbal Leave Policy rules.
 * These define the sit-in/make-up rules for each course type.
 */
export const LEAVE_POLICY_COURSE_RULES: LeavePolicyCourseRule[] = [
  // ── Beginner — Section 1 (Reading) ────────────────────────────────────────
  {
    id: "beginner-sec1-reading",
    courseName: "SAT Verbal Rank 3-Section 1",
    subject: "Reading",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "Section 1 (Reading) absence → makeup in another Rank 3 section, same lesson number only.",
    makeupRules: [
      "Section 2 (Writing) — same lesson #",
      "Section 3 (Math) — same lesson #",
    ],
    lastClassExcluded: true,
    makeupTargets: [
      { section: "Section 2", subject: "Writing" },
      { section: "Section 3", subject: "Math" },
    ],
    eligibleTargets: ["Section 2", "Section 3"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Another Rank 3 section (same lesson #)",
        makeupTargets: [
          { section: "Section 2", subject: "Writing" },
          { section: "Section 3", subject: "Math" },
        ],
      },
    ],
  },
  // ── Beginner — Section 2 (Writing) ────────────────────────────────────────
  {
    id: "beginner-sec2-writing",
    courseName: "SAT Verbal Rank 3-Section 2",
    subject: "Writing",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "Section 2 (Writing) absence → makeup in Section 1 (Reading), same lesson number only.",
    makeupRules: ["Section 1 (Reading) — same lesson #"],
    lastClassExcluded: true,
    makeupTargets: [{ section: "Section 1", subject: "Reading" }],
    eligibleTargets: ["Section 1"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Section 1 (Reading) — same lesson #",
        makeupTargets: [{ section: "Section 1", subject: "Reading" }],
      },
    ],
  },
  // ── Beginner — Section 3 (Math) ───────────────────────────────────────────
  {
    id: "beginner-sec3-math",
    courseName: "SAT Verbal Rank 3-Section 3",
    subject: "Math",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "Section 3 (Math) absence → makeup in Section 1 (Reading), same lesson number only.",
    makeupRules: ["Section 1 (Reading) — same lesson #"],
    lastClassExcluded: true,
    makeupTargets: [{ section: "Section 1", subject: "Reading" }],
    eligibleTargets: ["Section 1"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Section 1 (Reading) — same lesson #",
        makeupTargets: [{ section: "Section 1", subject: "Reading" }],
      },
    ],
  },

  // ── Rank 5 Reading (2 priorities) ───────────────────────────────────────
  {
    id: "sat-verbal-reading-rank5",
    courseName: "SAT Verbal Reading Rank 5",
    subject: "Reading",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Verbal Reading Rank 4. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Verbal Reading Rank 4 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Reading Rank 4", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Verbal Reading Rank 4",
        eligibleTargets: ["SAT Verbal Reading Rank 4"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  // ── Rank 5 Writing (2 priorities) ──────────────────────────────────────
  {
    id: "sat-verbal-writing-rank5",
    courseName: "SAT Verbal Writing Rank 5",
    subject: "Writing",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Verbal Writing Rank 4. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Verbal Writing Rank 4 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Writing Rank 4", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Verbal Writing Rank 4",
        eligibleTargets: ["SAT Verbal Writing Rank 4"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  {
    id: "sat-reading-rank5",
    courseName: "SAT Reading Rank 5",
    subject: "Reading",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Reading Rank 4. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Reading Rank 4 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Reading Rank 4", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Reading Rank 4",
        eligibleTargets: ["SAT Reading Rank 4"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  {
    id: "sat-writing-rank5",
    courseName: "SAT Writing Rank 5",
    subject: "Writing",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Writing Rank 4. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Writing Rank 4 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Writing Rank 4", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Writing Rank 4",
        eligibleTargets: ["SAT Writing Rank 4"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },

  // ── Rank 4 Reading (2 priorities) ───────────────────────────────────────
  {
    id: "sat-verbal-reading-rank4",
    courseName: "SAT Verbal Reading Rank 4",
    subject: "Reading",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Verbal Reading Rank 5. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Verbal Reading Rank 5 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Reading Rank 5", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Verbal Reading Rank 5",
        eligibleTargets: ["SAT Verbal Reading Rank 5"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  // ── Rank 4 Writing (2 priorities) ──────────────────────────────────────
  {
    id: "sat-verbal-writing-rank4",
    courseName: "SAT Verbal Writing Rank 4",
    subject: "Writing",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Verbal Writing Rank 5. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Verbal Writing Rank 5 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Writing Rank 5", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Verbal Writing Rank 5",
        eligibleTargets: ["SAT Verbal Writing Rank 5"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  {
    id: "sat-reading-rank4",
    courseName: "SAT Reading Rank 4",
    subject: "Reading",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Reading Rank 5. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Reading Rank 5 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Reading Rank 5", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Reading Rank 5",
        eligibleTargets: ["SAT Reading Rank 5"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },
  {
    id: "sat-writing-rank4",
    courseName: "SAT Writing Rank 4",
    subject: "Writing",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: SAT Writing Rank 5. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: SAT Writing Rank 5 — any available date",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Writing Rank 5", "SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: SAT Writing Rank 5",
        eligibleTargets: ["SAT Writing Rank 5"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: ["SAT Verbal Rank 3 Section 1", "SAT Verbal Rank 3 Section 2"],
      },
    ],
  },

  // ── Rank 3 — Section 1 (Reading) — 3 priorities ────────────────────────
  {
    id: "rank3-sec1-reading",
    courseName: "SAT Verbal Rank 3-Section 1",
    subject: "Reading",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "1st: Another Rank 3 section (same lesson #). 2nd: Rank 2. 3rd: Rank 4 Reading or Writing.",
    makeupRules: [
      "1st Priority: Another Rank 3 section — same lesson #",
      "2nd Priority: Rank 2",
      "3rd Priority: SAT Verbal Reading Rank 4 or Writing Rank 4 — any available date",
    ],
    lastClassExcluded: true,
    makeupTargets: [
      { section: "Section 2", subject: "Writing" },
      { section: "Section 3", subject: "Math" },
    ],
    eligibleTargets: ["Section 2", "Section 3", "SAT Verbal Rank 2", "SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Another Rank 3 section (same lesson #)",
        makeupTargets: [
          { section: "Section 2", subject: "Writing" },
          { section: "Section 3", subject: "Math" },
        ],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 2",
        eligibleTargets: ["SAT Verbal Rank 2"],
      },
      {
        level: 3,
        ruleType: "rank_chain",
        label: "3rd Priority: Rank 4 Reading or Writing",
        eligibleTargets: ["SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
      },
    ],
  },
  // ── Rank 3 — Section 2 (Writing) — 3 priorities (no Rank 2 — same time) ─
  {
    id: "rank3-sec2-writing",
    courseName: "SAT Verbal Rank 3-Section 2",
    subject: "Writing",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "1st: Another Rank 3 section (same lesson #). 2nd: (none — Rank 2 same time). 3rd: Rank 4 Reading or Writing.",
    makeupRules: [
      "1st Priority: Another Rank 3 section — same lesson #",
      "2nd Priority: (none — Rank 2 shares same time slot)",
      "3rd Priority: SAT Verbal Reading Rank 4 or Writing Rank 4 — any available date",
    ],
    lastClassExcluded: true,
    makeupTargets: [{ section: "Section 1", subject: "Reading" }],
    eligibleTargets: ["Section 1", "SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Another Rank 3 section (same lesson #)",
        makeupTargets: [{ section: "Section 1", subject: "Reading" }],
      },
      {
        level: 3,
        ruleType: "rank_chain",
        label: "3rd Priority: Rank 4 Reading or Writing",
        eligibleTargets: ["SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
      },
    ],
  },
  // ── Rank 3 — Section 3 (Math) — 3 priorities ───────────────────────────
  {
    id: "rank3-sec3-math",
    courseName: "SAT Verbal Rank 3-Section 3",
    subject: "Math",
    ruleType: "cross_section",
    priorityCount: 3,
    description:
      "1st: Another Rank 3 section (same lesson #). 2nd: Rank 2. 3rd: Rank 4 Reading or Writing.",
    makeupRules: [
      "1st Priority: Another Rank 3 section — same lesson #",
      "2nd Priority: Rank 2",
      "3rd Priority: SAT Verbal Reading Rank 4 or Writing Rank 4 — any available date",
    ],
    lastClassExcluded: true,
    makeupTargets: [{ section: "Section 1", subject: "Reading" }],
    eligibleTargets: ["Section 1", "SAT Verbal Rank 2", "SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
    priorities: [
      {
        level: 1,
        ruleType: "cross_section",
        label: "1st Priority: Another Rank 3 section (same lesson #)",
        makeupTargets: [{ section: "Section 1", subject: "Reading" }],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 2",
        eligibleTargets: ["SAT Verbal Rank 2"],
      },
      {
        level: 3,
        ruleType: "rank_chain",
        label: "3rd Priority: Rank 4 Reading or Writing",
        eligibleTargets: ["SAT Verbal Reading Rank 4", "SAT Verbal Writing Rank 4"],
      },
    ],
  },

  // ── Rank 2 (3 priorities) ──────────────────────────────────────────────
  {
    id: "sat-verbal-rank2",
    courseName: "SAT Verbal Rank 2",
    subject: "Rank 2",
    ruleType: "rank_chain",
    priorityCount: 3,
    description:
      "1st: Rank 3. 2nd: Rank 1. 3rd: Reading Mastery or Writing Wisdom (if open).",
    makeupRules: [
      "1st Priority: SAT Verbal Rank 3",
      "2nd Priority: SAT Verbal Rank 1",
      "3rd Priority: Reading Mastery or Writing Wisdom (may not open each cycle)",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 1", "Reading Mastery", "Writing Wisdom"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Rank 3",
        eligibleTargets: ["SAT Verbal Rank 3"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 1",
        eligibleTargets: ["SAT Verbal Rank 1"],
      },
      {
        level: 3,
        ruleType: "rank_chain",
        label: "3rd Priority: Reading Mastery or Writing Wisdom",
        eligibleTargets: ["Reading Mastery", "Writing Wisdom"],
      },
    ],
  },

  // ── Rank 1 (2 priorities) ──────────────────────────────────────────────
  {
    id: "sat-verbal-rank1",
    courseName: "SAT Verbal Rank 1",
    subject: "Rank 1",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: Rank 2. 2nd: Reading Mastery or Writing Wisdom (if open).",
    makeupRules: [
      "1st Priority: SAT Verbal Rank 2",
      "2nd Priority: Reading Mastery or Writing Wisdom (may not open each cycle)",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 2", "Reading Mastery", "Writing Wisdom"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Rank 2",
        eligibleTargets: ["SAT Verbal Rank 2"],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Reading Mastery or Writing Wisdom",
        eligibleTargets: ["Reading Mastery", "Writing Wisdom"],
      },
    ],
  },

  // ── Mastery / Wisdom (2 priorities, bidirectional) ─────────────────────
  {
    id: "reading-mastery",
    courseName: "Reading Mastery",
    subject: "Reading",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Rank 3 ↔ Rank 2. Direction depends on student's current rank.",
    makeupRules: [
      "If student is Rank 3 → makeup in Rank 2",
      "If student is Rank 2 → makeup in Rank 3",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 3 ↔ Rank 2)",
        eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
      },
    ],
  },
  {
    id: "writing-wisdom",
    courseName: "Writing Wisdom",
    subject: "Writing",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Rank 3 ↔ Rank 2. Direction depends on student's current rank.",
    makeupRules: [
      "If student is Rank 3 → makeup in Rank 2",
      "If student is Rank 2 → makeup in Rank 3",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 3 ↔ Rank 2)",
        eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
      },
    ],
  },

  // ── Real-Time Practice (1 priority, based on main course rank) ─────────
  {
    id: "sat-verbal-realtime-practice",
    courseName: "SAT Verbal Real-Time Practice",
    subject: "Practice",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Based on student's main course rank: Rank 3→2, Rank 2→1, Rank 1→2.",
    makeupRules: [
      "Rank 3 students → Rank 2",
      "Rank 2 students → Rank 1",
      "Rank 1 students → Rank 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: [
      "SAT Verbal Rank 3",
      "SAT Verbal Rank 2",
      "SAT Verbal Rank 1",
    ],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Based on main course rank",
        eligibleTargets: [
          "SAT Verbal Rank 3",
          "SAT Verbal Rank 2",
          "SAT Verbal Rank 1",
        ],
      },
    ],
  },

  // ── Brush Up (2 priorities) ────────────────────────────────────────────
  {
    id: "sat-verbal-brushup",
    courseName: "SAT Verbal Brush Up",
    subject: "Brush Up",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "1st: Opposite rank between Rank 4 and Rank 5. 2nd: Rank 3 Section 1 or Section 2.",
    makeupRules: [
      "1st Priority: Rank 4 → Rank 5 (R or W). Rank 5 → Rank 4 (R or W).",
      "2nd Priority: Rank 3 Section 1 or Section 2",
    ],
    lastClassExcluded: true,
    eligibleTargets: [
      "SAT Verbal Rank 4",
      "SAT Verbal Rank 5",
      "SAT Verbal Rank 3 Section 1",
      "SAT Verbal Rank 3 Section 2",
    ],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 4 ↔ Rank 5)",
        eligibleTargets: [
          "SAT Verbal Rank 4",
          "SAT Verbal Rank 5",
        ],
      },
      {
        level: 2,
        ruleType: "rank_chain",
        label: "2nd Priority: Rank 3 Section 1 or Section 2",
        eligibleTargets: [
          "SAT Verbal Rank 3 Section 1",
          "SAT Verbal Rank 3 Section 2",
        ],
      },
    ],
  },

  // ── Knock Out (2 priorities, bidirectional) ────────────────────────────
  {
    id: "sat-verbal-knockout",
    courseName: "SAT Verbal Knock Out",
    subject: "Knock Out",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Rank 3 ↔ Rank 2. Direction depends on student's current rank.",
    makeupRules: [
      "If student is Rank 3 → makeup in Rank 2",
      "If student is Rank 2 → makeup in Rank 3",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 3 ↔ Rank 2)",
        eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
      },
    ],
  },

  // ── Intensive (2 priorities, bidirectional) ────────────────────────────
  {
    id: "sat-verbal-intensive",
    courseName: "SAT Verbal Intensive",
    subject: "Intensive",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Rank 3 ↔ Rank 2. Direction depends on student's current rank.",
    makeupRules: [
      "If student is Rank 3 → makeup in Rank 2",
      "If student is Rank 2 → makeup in Rank 3",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 3 ↔ Rank 2)",
        eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
      },
    ],
  },

  // ── Believe (2 priorities, bidirectional) ──────────────────────────────
  {
    id: "sat-verbal-believe",
    courseName: "SAT Verbal Believe",
    subject: "Believe",
    ruleType: "rank_chain",
    priorityCount: 2,
    description:
      "Rank 3 ↔ Rank 2. Direction depends on student's current rank.",
    makeupRules: [
      "If student is Rank 3 → makeup in Rank 2",
      "If student is Rank 2 → makeup in Rank 3",
    ],
    lastClassExcluded: true,
    eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
    priorities: [
      {
        level: 1,
        ruleType: "rank_chain",
        label: "1st Priority: Opposite rank (Rank 3 ↔ Rank 2)",
        eligibleTargets: ["SAT Verbal Rank 3", "SAT Verbal Rank 2"],
      },
    ],
  },
];

/**
 * Extract rank number from a course name.
 * E.g. "SAT Verbal Rank 3" → 3, "SAT Verbal Reading Rank 5" → 5
 * Returns null if no rank found.
 */
function extractRankFromCourseName(courseName: string): number | null {
  const match = courseName.match(/Rank\s+(\d+)/i);
  return match ? parseInt(match[1], 10) : null;
}

/**
 * Extract section from a course name.
 * E.g. "SAT Verbal Rank 3 Section 2" → "Section 2"
 * Returns null if no section found.
 */
function extractSectionFromCourseName(courseName: string): string | null {
  const match = courseName.match(/Section\s+(\d+)/i);
  return match ? `Section ${match[1]}` : null;
}

/**
 * Evaluate a leave policy test input and return available makeup options.
 * Supports multi-priority rules via the `priorities` array.
 * @param maxPriorityToShow - For stepped reveal: only show priorities up to this level.
 */
export function evaluateLeavePolicy(
  input: LeavePolicyTestInput,
  rules: LeavePolicyCourseRule[] = LEAVE_POLICY_COURSE_RULES,
  maxPriorityToShow: number = 1
): LeavePolicyTestResult {
  const rule = rules.find((r) => r.id === input.courseRuleId);

  if (!rule) {
    return {
      input,
      options: [],
      isBlocked: true,
      blockReason: "Unknown course rule",
    };
  }

  // Universal block: last class cannot be made up
  if (input.isLastClass && rule.lastClassExcluded) {
    return {
      input,
      options: [],
      isBlocked: true,
      blockReason:
        "Last class of the cycle cannot be made up (End-of-class Meal day)",
    };
  }

  // Derive student's current rank from their course name
  const studentRank = extractRankFromCourseName(input.missedCourseName);

  const options: MakeupOption[] = [];

  // Use priorities array if defined (new multi-priority system)
  if (rule.priorities && rule.priorities.length > 0) {
    for (const priority of rule.priorities) {
      // Stepped reveal: skip priorities above maxPriorityToShow
      if (priority.level > maxPriorityToShow) continue;

      const levelLabel =
        priority.level === 1 ? "1st" : priority.level === 2 ? "2nd" : "3rd";

      switch (priority.ruleType) {
        case "cross_section": {
          if (priority.makeupTargets) {
            for (const target of priority.makeupTargets) {
              options.push({
                label: `${target.section} (${target.subject}) — same lesson #`,
                available: true,
                reason: `${levelLabel} Priority`,
              });
            }
          }
          break;
        }

        case "rank_chain": {
          if (priority.eligibleTargets) {
            const filtered = filterTargetsByStudentRank(
              priority.eligibleTargets,
              studentRank,
              rule.id
            );
            for (const target of filtered) {
              const targetRank = extractRankFromCourseName(target);
              options.push({
                label: target,
                available: !input.isLastClass,
                reason: input.isLastClass
                  ? "Last class excluded"
                  : `${levelLabel} Priority${
                      targetRank !== null && studentRank !== null
                        ? ` (Rank ${studentRank} → Rank ${targetRank})`
                        : ""
                    }`,
              });
            }
          }
          break;
        }

        case "any_day_except_last": {
          if (priority.anyDay) {
            options.push({
              label: "Any available session (except last class)",
              available: !input.isLastClass,
              reason: `${levelLabel} Priority`,
            });
          }
          break;
        }

        case "mastery_wisdom_choice": {
          options.push({
            label: "R Mastery (Recommended)",
            available: !input.isLastClass,
            reason: input.isLastClass ? "Last class excluded" : `${levelLabel} Priority`,
          });
          options.push({
            label: "W Wisdom (Not recommended — may not open each cycle)",
            available: !input.isLastClass,
            reason: input.isLastClass ? "Last class excluded" : `${levelLabel} Priority`,
          });
          break;
        }
      }
    }
  } else {
    // Fallback: use legacy single ruleType evaluation
    const legacyOptions = evaluateLegacy(rule, input, studentRank);
    options.push(...legacyOptions);
  }

  return {
    input,
    options,
    isBlocked: false,
  };
}

/**
 * Filter eligible targets based on student's current rank.
 * Handles special cases for specific rules.
 */
function filterTargetsByStudentRank(
  targets: string[],
  studentRank: number | null,
  ruleId: string
): string[] {
  if (studentRank === null) return targets;

  return targets.filter((target) => {
    const targetRank = extractRankFromCourseName(target);

    // Real-Time Practice: Rank 3→2, Rank 2→1, Rank 1→2
    if (ruleId === "sat-verbal-realtime-practice") {
      if (studentRank === 3) return targetRank === 2;
      if (studentRank === 2) return targetRank === 1;
      if (studentRank === 1) return targetRank === 2;
      return false;
    }

    // Brush Up: Rank 4↔5, Rank 3 → Sec 1 or Sec 2
    if (ruleId === "sat-verbal-brushup") {
      if (studentRank === 4) return targetRank === 5;
      if (studentRank === 5) return targetRank === 4;
      if (studentRank === 3) {
        const targetSection = extractSectionFromCourseName(target);
        return targetSection !== null;
      }
      return false;
    }

    // Rank 2: can go to Rank 3, Rank 1, or Mastery/Wisdom
    if (ruleId === "sat-verbal-rank2") {
      if (targetRank === 3) return true; // Rank 3
      if (targetRank === 1) return true; // Rank 1
      if (target === "Reading Mastery" || target === "Writing Wisdom") return true;
      return false;
    }

    // Mastery/Wisdom: bidirectional Rank 3 ↔ Rank 2
    if (ruleId === "reading-mastery" || ruleId === "writing-wisdom") {
      if (studentRank === 3) return targetRank === 2;
      if (studentRank === 2) return targetRank === 3;
      return false;
    }

    // Knock Out, Intensive, Believe: bidirectional Rank 3 ↔ Rank 2
    if (
      ruleId === "sat-verbal-knockout" ||
      ruleId === "sat-verbal-intensive" ||
      ruleId === "sat-verbal-believe"
    ) {
      if (studentRank === 3) return targetRank === 2;
      if (studentRank === 2) return targetRank === 3;
      return false;
    }

    // Rank 3 rules: can go to Rank 2, Rank 4, or other sections
    if (ruleId.startsWith("rank3-")) {
      // Rank 2 target
      if (target.includes("Rank 2") && studentRank === 3) return true;
      // Rank 4 target
      if (target.includes("Rank 4")) return true;
      // Other sections (cross_section targets)
      if (targetRank === null) return true;
      return false;
    }

    // Default: keep targets where targetRank !== studentRank
    if (targetRank === null) return true;
    return targetRank !== studentRank;
  });
}

/**
 * Legacy evaluation for rules without priorities array.
 */
function evaluateLegacy(
  rule: LeavePolicyCourseRule,
  input: LeavePolicyTestInput,
  studentRank: number | null
): MakeupOption[] {
  const options: MakeupOption[] = [];

  switch (rule.ruleType) {
    case "cross_section": {
      if (rule.makeupTargets && rule.makeupTargets.length > 0) {
        for (let i = 0; i < rule.makeupTargets.length; i++) {
          const target = rule.makeupTargets[i];
          options.push({
            label: `${target.section} (${target.subject}) — Occurrence #${input.missedOccurrence}`,
            available: true,
            reason: i === 0 ? "1st Priority" : `${i + 1}${i === 1 ? "nd" : "rd"} Priority`,
          });
        }
      }
      break;
    }

    case "any_day_except_last": {
      options.push({
        label: "Any available session (except last class)",
        available: true,
        reason: input.isLastClass ? "Blocked: last class" : "Available",
      });
      for (let p = 1; p <= rule.priorityCount; p++) {
        options.push({
          label: `${p}${p === 1 ? "st" : p === 2 ? "nd" : "rd"} Priority Make-up`,
          available: !input.isLastClass,
          reason: input.isLastClass ? "Last class excluded" : undefined,
        });
      }
      break;
    }

    case "rank_chain": {
      if (studentRank === null) {
        for (const target of rule.eligibleTargets) {
          options.push({
            label: target,
            available: !input.isLastClass,
            reason: input.isLastClass ? "Last class excluded" : "Available",
          });
        }
      } else {
        const filtered = filterTargetsByStudentRank(
          rule.eligibleTargets,
          studentRank,
          rule.id
        );
        for (const target of filtered) {
          const targetRank = extractRankFromCourseName(target);
          options.push({
            label: target,
            available: !input.isLastClass,
            reason: input.isLastClass
              ? "Last class excluded"
              : `Rank ${studentRank} student → Rank ${targetRank}`,
          });
        }
      }
      break;
    }

    case "mastery_wisdom_choice": {
      options.push({
        label: "R Mastery (Recommended)",
        available: !input.isLastClass,
        reason: input.isLastClass ? "Last class excluded" : "1st Priority",
      });
      options.push({
        label: "W Wisdom (Not recommended — may not open each cycle)",
        available: !input.isLastClass,
        reason: input.isLastClass ? "Last class excluded" : "2nd Priority",
      });
      break;
    }
  }

  return options;
}

/**
 * Get rule type badge color for UI rendering.
 */
export function getRuleTypeBadgeColor(
  ruleType: LeavePolicyCourseRule["ruleType"]
): { bg: string; text: string } {
  switch (ruleType) {
    case "cross_section":
      return { bg: "bg-blue-50", text: "text-blue-700" };
    case "any_day_except_last":
      return { bg: "bg-green-50", text: "text-green-700" };
    case "rank_chain":
      return { bg: "bg-purple-50", text: "text-purple-700" };
    case "mastery_wisdom_choice":
      return { bg: "bg-amber-50", text: "text-amber-700" };
    default:
      return { bg: "bg-gray-50", text: "text-gray-700" };
  }
}

/**
 * Get human-readable rule type label.
 */
export function getRuleTypeLabel(
  ruleType: LeavePolicyCourseRule["ruleType"]
): string {
  switch (ruleType) {
    case "cross_section":
      return "Cross-Section";
    case "any_day_except_last":
      return "Any Day (Except Last)";
    case "rank_chain":
      return "Rank Chain";
    case "mastery_wisdom_choice":
      return "Mastery/Wisdom Choice";
    default:
      return ruleType;
  }
}
