import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ApiRequestError, newIdempotencyKey } from "@/api/client";
import Button from "@/components/ui/Button";
import SearchInput from "@/components/ui/SearchInput";
import { useToast } from "@/hooks/useToast";
import { queryClient } from "@/query/cache";
import {
  createCourseGroup,
  dismissCourseLinkSuggestion,
  getCourseLinkSuggestions,
} from "@/features/courses/api/courseApi";
import type { CourseLinkSuggestion, CourseLinkSuggestionCourse, CourseLinkSuggestionsResponse } from "@/features/courses/types";

const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

const REASON_LABELS: Record<string, string> = {
  same_subject: "Same subject",
  normalized_name_match: "Names match after normalization",
  same_teaching_period: "Same configured teaching period",
  period_unknown: "Teaching period is not confirmed",
  compatible_slots: "Weekly slots match",
  slot_mismatch: "Weekly slots differ",
  equal_slot_durations: "Session durations match",
  duration_mismatch: "Session durations differ",
  complementary_dates: "Sessions occur on different dates",
  same_date_sessions: "Sessions share a local date",
  overlapping_sessions: "Session times overlap",
  unique_partner: "Only possible matching partner",
  multiple_possible_partners: "More than one possible partner",
};

// Codes that weaken a match; everything else supports it.
const REVIEW_REASONS = new Set([
  "period_unknown",
  "slot_mismatch",
  "duration_mismatch",
  "same_date_sessions",
  "overlapping_sessions",
  "multiple_possible_partners",
]);

function slotLabel(slot: string): string {
  const [weekdayRaw, time, secondsRaw] = slot.split("|");
  const weekday = Number(weekdayRaw);
  const seconds = Number(secondsRaw);
  const duration = Number.isFinite(seconds) ? `${Math.round(seconds / 60)} min` : "duration unknown";
  return `${WEEKDAYS[weekday - 1] ?? `Day ${weekdayRaw}`} ${time ?? ""} (${duration})`;
}

function dateRange(course: CourseLinkSuggestionCourse): string {
  if (!course.session_date_from || !course.session_date_to) return "Dates unavailable";
  if (course.session_date_from === course.session_date_to) return course.session_date_from;
  return `${course.session_date_from} – ${course.session_date_to}`;
}

function teacherSummary(teachers: string[]): string {
  return teachers.length > 0 ? teachers.join(", ") : "No teacher assignment";
}

function courseConfiguration(course: CourseLinkSuggestionCourse): string {
  return [
    course.level === null ? "Level not set" : `Level ${course.level}`,
    course.cycle_label ?? "Cycle not set",
    course.root_course_group_name ?? "Root course group not set",
  ].join(" · ");
}

function effectiveConsequences(course: CourseLinkSuggestionCourse): string {
  const absence = course.absence_form_active
    ? "absence form active"
    : course.absence_form_visible
      ? "absence form visible but inactive"
      : "absence form hidden";
  const sitInRule = course.sit_in_rule_name
    ? `Sit-in rule: ${course.sit_in_rule_name}${course.sit_in_rule_type ? ` (${course.sit_in_rule_type})` : ""}`
    : "No sit-in rule";
  return [sitInRule, course.sit_in_rule_description, absence].filter(Boolean).join(" · ");
}

function suggestionKey(item: CourseLinkSuggestion): string {
  return `${item.configured_source.id}:${item.unconfigured_course.id}`;
}

function generatedGroupName(item: CourseLinkSuggestion): string {
  return `Same course: ${item.configured_source.code} + ${item.unconfigured_course.code}`;
}

function manualLinkHref(item: CourseLinkSuggestion): string {
  const source = item.configured_source.id;
  const params = new URLSearchParams({
    mode: "merge",
    continuation: "1",
    courses: `${source},${item.unconfigured_course.id}`,
    source,
    name: generatedGroupName(item),
  });
  return `/courses/create?${params.toString()}`;
}

export default function CourseLinkSuggestionsPanel() {
  const { addToast } = useToast();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const requestRef = useRef(0);
  const mutationKeys = useRef(new Map<string, string>());
  const rowRefs = useRef(new Map<string, HTMLElement>());
  const [includeReview, setIncludeReview] = useState(false);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [response, setResponse] = useState<CourseLinkSuggestionsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [busy, setBusy] = useState<{ key: string; action: "confirm" | "dismiss" } | null>(null);
  const [announcement, setAnnouncement] = useState("");

  const loadFirstPage = useCallback(async () => {
    const requestID = ++requestRef.current;
    setLoading(true);
    setLoadError(null);
    try {
      const result = await getCourseLinkSuggestions({ includeReview, search });
      if (requestID !== requestRef.current) return;
      setResponse(result);
    } catch (error) {
      if (requestID !== requestRef.current) return;
      setLoadError(error instanceof Error ? error.message : "Suggestions are unavailable.");
    } finally {
      if (requestID === requestRef.current) setLoading(false);
    }
  }, [includeReview, search]);

  useEffect(() => {
    const value = searchInput.trim();
    if (value === search) return;
    const timer = setTimeout(() => setSearch(value), 300);
    return () => clearTimeout(timer);
  }, [searchInput, search]);

  useEffect(() => {
    void loadFirstPage();
    return () => {
      requestRef.current += 1;
    };
  }, [loadFirstPage]);

  async function loadMore() {
    if (!response?.next_cursor || loadingMore) return;
    const requestID = requestRef.current;
    setLoadingMore(true);
    try {
      const nextPage = await getCourseLinkSuggestions({ includeReview, search, cursor: response.next_cursor });
      if (requestID !== requestRef.current) return;
      setResponse((current) => current ? {
        ...nextPage,
        items: [...current.items, ...nextPage.items],
      } : nextPage);
    } catch (error) {
      if (requestID !== requestRef.current) return;
      setRefreshError(error instanceof Error ? error.message : "More suggestions could not be loaded.");
    } finally {
      setLoadingMore(false);
    }
  }

  async function refreshAfterDecision() {
    const requestID = ++requestRef.current;
    try {
      const result = await getCourseLinkSuggestions({ includeReview, search });
      if (requestID !== requestRef.current) return;
      setResponse(result);
      setLoadError(null);
      setRefreshError(null);
    } catch {
      if (requestID !== requestRef.current) return;
      setRefreshError("Your decision was saved, but the suggestions could not be refreshed.");
    }
  }

  function removeRow(item: CourseLinkSuggestion, message: string) {
    const key = suggestionKey(item);
    // Move focus to the row that takes the removed row's place, else the heading.
    let nextKey: string | null = null;
    setResponse((current) => {
      if (!current) return current;
      const index = current.items.findIndex((entry) => suggestionKey(entry) === key);
      const items = current.items.filter((entry) => suggestionKey(entry) !== key);
      const next = items[Math.min(Math.max(index, 0), items.length - 1)];
      nextKey = next ? suggestionKey(next) : null;
      return { ...current, items };
    });
    setAnnouncement(message);
    requestAnimationFrame(() => {
      const row = nextKey ? rowRefs.current.get(nextKey) : undefined;
      (row ?? headingRef.current)?.focus();
    });
  }

  async function confirmSuggestion(item: CourseLinkSuggestion) {
    const key = suggestionKey(item);
    const actionKey = `${key}:${item.evidence_fingerprint}:confirm`;
    const idempotencyKey = mutationKeys.current.get(actionKey) ?? newIdempotencyKey();
    mutationKeys.current.set(actionKey, idempotencyKey);
    setBusy({ key, action: "confirm" });
    setAnnouncement("");
    try {
      const source = item.configured_source;
      const partial = item.unconfigured_course;
      await createCourseGroup({
        name: generatedGroupName(item),
        course_ids: [source.id, partial.id],
        kind: "continuation",
        rule_source_course_id: source.id,
        suggestion_precondition: {
          configured_course_id: source.id,
          unconfigured_course_id: partial.id,
          evidence_fingerprint: item.evidence_fingerprint,
          detector_version: response?.detector_version ?? "",
        },
      }, idempotencyKey);
      mutationKeys.current.delete(actionKey);
      removeRow(item, `${source.code} and ${partial.code} were linked as the same course.`);
      try {
        await queryClient.invalidateQueries({ queryKey: ["api", "/api/v1/course-groups"] });
      } catch {
        setRefreshError("The link was saved, but related course views could not be refreshed.");
      }
      await refreshAfterDecision();
    } catch (error) {
      if (error instanceof ApiRequestError && error.code === "suggestion_changed") {
        mutationKeys.current.delete(actionKey);
        setAnnouncement("This suggestion changed. The latest evidence is being loaded; review it before deciding again.");
        await refreshAfterDecision();
      } else {
        if (error instanceof ApiRequestError && error.code === "duplicate_group_name") {
          setRefreshError("That generated group name is already in use. Continue through manual course linking.");
        } else {
          addToast("error", error instanceof Error ? error.message : "The course link could not be saved.");
        }
      }
    } finally {
      setBusy(null);
    }
  }

  async function dismissSuggestion(item: CourseLinkSuggestion) {
    const key = suggestionKey(item);
    const actionKey = `${key}:${item.evidence_fingerprint}:dismiss`;
    const idempotencyKey = mutationKeys.current.get(actionKey) ?? newIdempotencyKey();
    mutationKeys.current.set(actionKey, idempotencyKey);
    setBusy({ key, action: "dismiss" });
    setAnnouncement("");
    try {
      await dismissCourseLinkSuggestion({
        configured_course_id: item.configured_source.id,
        unconfigured_course_id: item.unconfigured_course.id,
        evidence_fingerprint: item.evidence_fingerprint,
        detector_version: response?.detector_version ?? "",
      }, idempotencyKey);
      mutationKeys.current.delete(actionKey);
      removeRow(item, `Suggestion for ${item.configured_source.code} and ${item.unconfigured_course.code} dismissed.`);
      await refreshAfterDecision();
    } catch (error) {
      if (error instanceof ApiRequestError && error.code === "suggestion_changed") {
        mutationKeys.current.delete(actionKey);
        setAnnouncement("This suggestion changed. The latest evidence is being loaded; review it before deciding again.");
        await refreshAfterDecision();
      } else {
        addToast("error", error instanceof Error ? error.message : "The suggestion could not be dismissed.");
      }
    } finally {
      setBusy(null);
    }
  }

  if (response && !response.enabled) return null;

  return (
    <section className="mt-6 rounded-md border border-[var(--color-wi-line)] bg-white p-4 sm:p-5" aria-labelledby="course-link-suggestions-heading">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="course-link-suggestions-heading" ref={headingRef} tabIndex={-1} className="text-base font-semibold text-[var(--color-wi-text)]">
            Possible same course
          </h2>
          <p className="mt-1 max-w-3xl text-sm text-[var(--color-wi-text-light)]">
            These course IDs share a subject and normalized name. Teacher assignments are shown as context and do not affect matching.
          </p>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <div className="w-full sm:w-72">
          <SearchInput
            value={searchInput}
            onChange={setSearchInput}
            label="Search course link suggestions"
            placeholder="Course code, name, subject, or teacher"
          />
        </div>
        <label className="flex items-center gap-2 text-sm text-[var(--color-wi-text)]">
          <input
            type="checkbox"
            checked={includeReview}
            onChange={(event) => {
              requestRef.current += 1;
              setIncludeReview(event.target.checked);
            }}
          />
          Show review candidates
        </label>
      </div>

      {response?.mode === "discovery" ? (
        <p className="mt-3 rounded-sm bg-[var(--color-wi-callout)] px-3 py-2 text-sm text-[var(--color-wi-text-light)]">
          Discovery is read only. Link confirmation will be available after its release checks are complete.
        </p>
      ) : null}
      {/* Stays mounted so screen readers pick up text changes; toasts carry errors. */}
      <p className={announcement ? "mt-3 text-sm text-[var(--color-wi-text)]" : "sr-only"} role="status" aria-live="polite">{announcement}</p>
      {refreshError ? <p className="mt-2 text-sm text-[var(--color-wi-amber)]" role="alert">{refreshError}</p> : null}

      {loading ? <p className="mt-4 text-sm text-[var(--color-wi-text-light)]" role="status">Checking possible course links…</p> : null}
      {loadError ? (
        <p className="mt-4 rounded-sm border border-[var(--color-wi-line)] px-3 py-3 text-sm text-[var(--color-wi-text-light)]" role="alert">
          Suggestions are unavailable. Course level management is still available below. {loadError}
        </p>
      ) : null}

      {!loading && !loadError && response?.enabled && response.items.length === 0 ? (
        <p className="mt-4 text-sm text-[var(--color-wi-text-light)]">
          {search
            ? `No suggestions match “${search}”.`
            : includeReview ? "No possible links need review." : "No high-confidence course links found."}
        </p>
      ) : null}

      <div className="mt-4 space-y-3">
        {response?.items.map((item) => {
          const rowKey = suggestionKey(item);
          const source = item.configured_source;
          const partial = item.unconfigured_course;
          const busyAction = busy?.key === rowKey ? busy.action : null;
          const consequenceID = `course-link-consequence-${rowKey}`;
          const supports = item.reason_codes.filter((reason) => !REVIEW_REASONS.has(reason));
          const concerns = item.reason_codes.filter((reason) => REVIEW_REASONS.has(reason));
          return (
            <article
              key={rowKey}
              ref={(node) => {
                if (node) rowRefs.current.set(rowKey, node);
                else rowRefs.current.delete(rowKey);
              }}
              tabIndex={-1}
              className="rounded-md border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)]/40 p-4 outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]" aria-label={`${item.confidence === "high" ? "High confidence" : "Review"} course link: ${source.code} and ${partial.code}`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <span className={`rounded-full border px-2 py-1 text-xs font-medium ${item.confidence === "high" ? "border-[var(--color-wi-green)]/30 bg-white text-[var(--color-wi-green)]" : "border-[var(--color-wi-amber)]/30 bg-white text-[var(--color-wi-amber)]"}`}>
                    {item.confidence === "high" ? "High confidence" : "Review"}
                  </span>
                  {item.ambiguity_count > 1 ? <span className="text-xs text-[var(--color-wi-amber)]">{item.ambiguity_count} possible partners</span> : null}
                </div>
              </div>

              <div className="mt-3 grid gap-3 md:grid-cols-2">
                {[source, partial].map((course, index) => (
                  <div key={course.id} className="rounded-sm border border-[var(--color-wi-line)] bg-white p-3">
                    <p className="text-xs font-medium uppercase tracking-wide text-[var(--color-wi-faint)]">
                      {index === 0 ? "Configured source · its rules will be used" : "Possible split course"}
                    </p>
                    <p className="mt-1 font-semibold text-[var(--color-wi-text)]">{course.code} · {course.name}</p>
                    <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">Course ID <span className="font-mono">{course.id}</span></p>
                    <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">{course.subject_code} · {course.subject_name}</p>
                    <p className="text-sm text-[var(--color-wi-text-light)]">{courseConfiguration(course)}</p>
                    {index === 0 ? <p className="text-sm text-[var(--color-wi-text-light)]">Effective rules: {effectiveConsequences(course)}</p> : null}
                    <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">{course.session_count} sessions · {dateRange(course)}</p>
                    <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">Teachers: {teacherSummary(course.teachers)}</p>
                    <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">
                      Slots: {course.slots.length > 0 ? course.slots.map(slotLabel).join(", ") : "Unavailable"}
                    </p>
                  </div>
                ))}
              </div>

              <div className="mt-3 grid gap-3 md:grid-cols-2">
                {[
                  { title: "Supports a match", reasons: supports, tone: "text-[var(--color-wi-text-light)]" },
                  { title: "Needs review", reasons: concerns, tone: "text-[var(--color-wi-amber)]" },
                ].filter((group) => group.reasons.length > 0).map((group) => (
                  <div key={group.title}>
                    <p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">{group.title}</p>
                    <ul className="mt-1 flex flex-wrap gap-2" aria-label={group.title}>
                      {group.reasons.map((reason) => (
                        <li key={reason} className={`rounded-sm bg-white px-2 py-1 text-xs ${group.tone}`}>
                          {REASON_LABELS[reason] ?? reason}
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>

              <div className="mt-4 flex flex-wrap items-center gap-2">
                {item.confidence === "high" && response.confirmation_enabled ? (
                  <Button size="sm" loading={busyAction === "confirm"} disabled={busy !== null} aria-describedby={consequenceID} onClick={() => void confirmSuggestion(item)}>
                    Link as same course
                  </Button>
                ) : null}
                {response.confirmation_enabled ? (
                  <Button variant="secondary" size="sm" loading={busyAction === "dismiss"} disabled={busy !== null} onClick={() => void dismissSuggestion(item)}>
                    Not the same
                  </Button>
                ) : null}
                {item.confidence === "review" ? (
                  <Link className="inline-flex min-h-[28px] items-center rounded-sm border border-wi-line bg-white px-2 py-1 text-xs font-medium text-[var(--color-wi-text)] underline-offset-2 hover:underline" to={manualLinkHref(item)}>
                    Review with manual linking
                  </Link>
                ) : null}
              </div>
              {item.confidence === "high" && response.confirmation_enabled ? (
                <p id={consequenceID} className="mt-3 text-xs leading-5 text-[var(--color-wi-text-light)]">
                  Linking shares enrollment and absence rules. Once absences use the link, it cannot be unlinked through this screen.
                </p>
              ) : null}
            </article>
          );
        })}
      </div>

      {response?.has_more ? (
        <div className="mt-4">
          <Button variant="secondary" size="sm" loading={loadingMore} onClick={() => void loadMore()}>
            Load more suggestions
          </Button>
        </div>
      ) : null}
    </section>
  );
}
