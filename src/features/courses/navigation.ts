export const COURSES_LIST_PATH = "/courses";

export type CourseDetailNavigationState = {
  returnTo: string;
};

export function createCourseDetailNavigationState(returnTo: string): CourseDetailNavigationState {
  return { returnTo };
}

export function getCoursesReturnPath(state: unknown): string {
  if (typeof state !== "object" || state === null || !("returnTo" in state)) {
    return COURSES_LIST_PATH;
  }

  const returnTo = state.returnTo;
  if (typeof returnTo !== "string") return COURSES_LIST_PATH;

  return returnTo === COURSES_LIST_PATH || returnTo.startsWith(`${COURSES_LIST_PATH}?`)
    ? returnTo
    : COURSES_LIST_PATH;
}
