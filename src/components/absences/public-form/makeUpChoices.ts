import type { SubjectSessions } from "@/types";
import { formatDate } from "@/utils/date";
import { combineSubjectGroups, groupByDay, mergedSessionValue, splitMergedSessionValue } from "@/features/absences/domain/sessionGrouping";
import { getAbsenceMessageCopy } from "@/features/absences/domain/absenceMessageCopy";
import {
  blockedSitInSessionIds,
  findSitInSessionConflicts,
  firstPriorityLevel,
  formatSitInDisplayDetails,
  formatSitInSessionConflictDescription,
  groupWithSitInForMissedSession,
  hasServerPriorityReveal,
  nextPriorityLevel,
  normalizeSitInDisplayModel,
  prioritiesForLevel,
  rootAvailableSessionsForMissedSessions,
  sitInOptionGroupsBySession,
  sitInOptionsByTargetAndSession,
  unavailableSessionsForMissedSessions,
  type SitInCourse,
  type SitInOptionGroup,
} from "@/features/absences/domain/sitInResolution";
import type { MakeUpOption } from "./MakeUpPicker";

type CurrentSitInOwner = { subjectName: string; date: string };

export function currentSitInOwners(
  groups: SubjectSessions[],
  selectedSubjectIds: string[],
  selectedSessionIds: Set<string>,
  sitInSelections: Record<string, string>,
  currentSessionIds: string[],
): Map<string, CurrentSitInOwner> {
  const current = new Set(currentSessionIds);
  const owners = new Map<string, CurrentSitInOwner>();
  for (const group of groups) {
    if (!selectedSubjectIds.includes(group.subject_id)) continue;
    for (const missed of group.sessions) {
      if (!selectedSessionIds.has(missed.id) || missed.already_absent || current.has(missed.id)) continue;
      for (const sessionId of splitMergedSessionValue(sitInSelections[missed.id])) {
        if (!owners.has(sessionId)) owners.set(sessionId, {
          subjectName: group.subject_name || group.course_name,
          date: formatDate(missed.date || missed.start_at),
        });
      }
    }
  }
  return owners;
}

export function makeUpPickerOptions(
  optionGroups: SitInOptionGroup[],
  sessions: SubjectSessions[],
  selectedSubjectIds: string[],
  groupLabel: string,
  defaultSitInCourse?: SitInCourse,
  selectedSitInSessionIds: string[] = [],
  currentValue = "",
  currentOwners: Map<string, CurrentSitInOwner> = new Map(),
  publicForm = false,
): MakeUpOption[] {
  const currentIds = new Set(splitMergedSessionValue(currentValue));
  const selectedIds = new Set(selectedSitInSessionIds);
  return optionGroups.map((optionGroup) => {
    const conflicts = findSitInSessionConflicts(optionGroup.items, sessions, selectedSubjectIds);
    const historical = optionGroup.items.find((item) => item.conflict)?.conflict;
    const duplicateOwner = optionGroup.items.map((item) => currentOwners.get(item.id)).find(Boolean);
    const duplicateSelection = Boolean(duplicateOwner) || optionGroup.items.some((item) => selectedIds.has(item.id) && !currentIds.has(item.id));
    const publicDescription = conflicts.length > 0
      ? "Overlaps another scheduled class"
      : historical
        ? "Already assigned to another absence"
        : duplicateSelection ? "Chosen for another absence in this form" : undefined;
    const description = publicForm ? publicDescription : conflicts.length > 0
      ? "You already have another class at this time."
      : historical ? getAbsenceMessageCopy("sit_in_session_already_used")
        : duplicateSelection ? "You already chose this make-up class for another absence. Choose a different time." : undefined;
    const conflictDetails = conflicts.length > 0
      ? formatSitInSessionConflictDescription(conflicts)?.replace(/^Overlaps with\s*/, "")
      : historical
        ? [historical.absence_subject_name, historical.absence_date_from ? formatDate(historical.absence_date_from) : undefined].filter(Boolean).join(" · ") || undefined
        : duplicateOwner ? `${duplicateOwner.subjectName} · ${duplicateOwner.date}` : undefined;
    const display = normalizeSitInDisplayModel(
      optionGroup.items, optionGroup.sitInCourse ?? defaultSitInCourse, groupLabel, sessions,
      description ? { status: "unavailable", message: description } : undefined,
    );
    return {
      value: mergedSessionValue(optionGroup.items), label: display.className,
      details: formatSitInDisplayDetails(display), display,
      disabled: Boolean(description), description, conflictDetails,
    };
  });
}

export type PublicMakeUpChoices = {
  options: MakeUpOption[];
  groupsByLevel: Map<number, SubjectSessions>;
  latestGroup: SubjectSessions;
  latestLevel: number;
  hasMore: boolean;
  unavailableMessage?: string;
};

// Browsing history and the committed choice are independent: revealing a batch
// never changes the priority used to review, save, or submit the chosen class.
export function publicMakeUpChoices({
  group, sessionIds, history, selectedLevel, sessions, selectedSubjectIds, selectedSessionIds, selections, label, blockedSessionIds,
}: {
  group: SubjectSessions;
  sessionIds: string[];
  history: Record<number, SubjectSessions> | undefined;
  selectedLevel: number | undefined;
  sessions: SubjectSessions[];
  selectedSubjectIds: string[];
  selectedSessionIds: Set<string>;
  selections: Record<string, string>;
  label: string;
  blockedSessionIds?: ReadonlySet<string>;
}): PublicMakeUpChoices {
  const sessionId = sessionIds[0];
  const source = groupWithSitInForMissedSession(group, sessionId);
  const baseLevel = source.sit_in?.current_priority_level ?? firstPriorityLevel(source);
  const levels = [...new Set([baseLevel, selectedLevel ?? baseLevel, ...Object.keys(history ?? {}).map(Number)])].sort((a, b) => a - b);
  const groupsByLevel = new Map<number, SubjectSessions>();
  const options = new Map<string, MakeUpOption>();
  const unavailableById = new Map<string, string>();
  const owners = currentSitInOwners(sessions, selectedSubjectIds, selectedSessionIds, selections, sessionIds);
  const activeIds = new Set(sessions.filter((item) => selectedSubjectIds.includes(item.subject_id))
    .flatMap((item) => item.sessions.filter((session) => selectedSessionIds.has(session.id) && !session.already_absent).map((session) => session.id)));
  const selectedSitInIds = Object.entries(selections).filter(([id]) => activeIds.has(id)).flatMap(([, value]) => splitMergedSessionValue(value));
  let unavailableMessage: string | undefined;

  for (const level of levels) {
    const batch = history?.[level] ?? source;
    groupsByLevel.set(level, batch);
    const priorities = prioritiesForLevel(batch, level);
    const optionGroups = (batch.sit_in?.priorities?.length ?? 0) > 0
      ? sitInOptionsByTargetAndSession(priorities, sessionIds)
      : sitInOptionGroupsBySession(rootAvailableSessionsForMissedSessions(batch.sit_in, sessionIds), batch.sit_in?.sit_in_course);
    const batchOptions = makeUpPickerOptions(optionGroups, sessions, selectedSubjectIds, label, batch.sit_in?.sit_in_course, selectedSitInIds, selections[sessionId], owners, true);
    for (const option of batchOptions) {
      for (const id of splitMergedSessionValue(option.value)) unavailableById.delete(id);
      const key = splitMergedSessionValue(option.value).sort().join("|");
      const selectedValue = selections[sessionId];
      const value = selectedValue && splitMergedSessionValue(selectedValue).sort().join("|") === key
        ? selectedValue : options.get(key)?.value ?? option.value;
      options.set(key, { ...option, value, priorityLevel: level });
    }
    const unavailable = priorities.length > 0
      ? priorities.flatMap((priority) => unavailableSessionsForMissedSessions(priority, sessionIds).map((item) => ({ item, course: priority.sit_in_course })))
      : unavailableSessionsForMissedSessions({ level, label: "", unavailable_sessions: batch.sit_in?.unavailable_sessions }, sessionIds).map((item) => ({ item, course: batch.sit_in?.sit_in_course }));
    for (const { item, course } of unavailable) {
      const message = item.reason_code === "sit_in_session_already_used" ? "Already assigned to another absence" : getAbsenceMessageCopy(item.reason_code);
      unavailableMessage = message;
      if (!item.session) continue;
      unavailableById.set(item.session.id, message);
      const display = normalizeSitInDisplayModel([item.session], course, label, sessions, { status: "unavailable", message });
      if (!options.has(item.session.id)) options.set(item.session.id, {
        value: item.session.id, label: display.className, details: formatSitInDisplayDetails(display), display,
        disabled: true, description: message, priorityLevel: level,
      });
    }
  }

  const blockedIds = blockedSitInSessionIds(sessions);
  for (const id of blockedSessionIds ?? []) blockedIds.add(id);
  for (const enrolled of sessions) {
    for (const sitIn of [enrolled.sit_in, ...Object.values(enrolled.sit_in?.sit_in_by_missed_session ?? {})]) {
      const candidates = [...(sitIn?.available_sessions ?? []), ...(sitIn?.priorities ?? []).flatMap((priority) => priority.available_sessions ?? [])];
      for (const candidate of candidates) if (candidate.conflict) blockedIds.add(candidate.id);
    }
  }
  const latestLevel = levels[levels.length - 1];
  const latestGroup = groupsByLevel.get(latestLevel) ?? source;
  return {
    options: [...options.values()].map((option) => {
      const ids = splitMergedSessionValue(option.value);
      const blocked = ids.some((id) => blockedIds.has(id));
      const unavailable = ids.map((id) => unavailableById.get(id)).find(Boolean);
      return blocked || unavailable ? {
        ...option, disabled: true,
        description: blocked ? "Already assigned to another absence" : unavailable,
      } : option;
    }),
    groupsByLevel, latestGroup, latestLevel,
    hasMore: hasServerPriorityReveal(latestGroup)
      ? Boolean(latestGroup.sit_in?.has_next_priority)
      : nextPriorityLevel(latestGroup, latestLevel) !== null,
    unavailableMessage,
  };
}

export function publicMakeUpChoicesBySession(input: {
  sessions: SubjectSessions[];
  selectedSubjectIds: string[];
  selectedSessionIds: Set<string>;
  selections: Record<string, string>;
  priorityLevels: Record<string, number>;
  priorityHistory: Record<string, Record<number, SubjectSessions>>;
  blockedSessionIds?: ReadonlySet<string>;
}): Map<string, PublicMakeUpChoices> {
  const choices = new Map<string, PublicMakeUpChoices>();
  const selectedGroups = input.sessions.filter((group) => input.selectedSubjectIds.includes(group.subject_id));
  for (const block of combineSubjectGroups(selectedGroups)) {
    for (const day of groupByDay(block.sessions)) {
      const active = day.items.filter((item) => !item.already_absent);
      const first = active[0];
      if (!first || !active.every((item) => input.selectedSessionIds.has(item.id))) continue;
      const group = block.groups.find((item) => item.sessions.some((session) => session.id === first.id)) ?? block.groups[0];
      if (groupWithSitInForMissedSession(group, first.id).sit_in?.sit_in_method !== "physical") continue;
      const model = publicMakeUpChoices({
        ...input, group, sessionIds: active.map((item) => item.id), label: block.label,
        history: input.priorityHistory[first.id], selectedLevel: input.priorityLevels[first.id],
      });
      for (const item of active) choices.set(item.id, model);
    }
  }
  return choices;
}
