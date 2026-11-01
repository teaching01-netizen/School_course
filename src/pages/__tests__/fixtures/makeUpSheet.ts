import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

/** The public make-up picker is a trigger plus a sheet at every width; these
 *  helpers drive it the way a student does instead of a hidden native select. */
export type MakeUpChoice = { value: string; name: string; disabled: boolean; checked: boolean };

export async function makeUpTriggers(): Promise<HTMLButtonElement[]> {
  await screen.findAllByRole("button", { name: /make-up class/i });
  return [...document.querySelectorAll<HTMLButtonElement>("[data-make-up-trigger]")];
}

export async function openMakeUpSheet(user: UserEvent, index = 0): Promise<HTMLElement> {
  const trigger = (await makeUpTriggers())[index];
  if (!trigger) throw new Error(`No make-up picker at index ${index}`);
  await user.click(trigger);
  return screen.findByRole("dialog", { name: "Choose a make-up class" });
}

function visibleChoices(dialog: HTMLElement): MakeUpChoice[] {
  return [...dialog.querySelectorAll<HTMLInputElement>('fieldset input[type="radio"]')]
    .filter((input) => input.value !== "")
    .map((input) => ({
      value: input.value,
      name: (input.closest("label")?.textContent ?? "").replace(/\s+/g, " ").trim(),
      disabled: input.disabled,
      checked: input.checked,
    }));
}

function dayChips(dialog: HTMLElement): HTMLInputElement[] {
  const strip = within(dialog).queryByRole("radiogroup", { name: "Make-up date" });
  return strip ? within(strip).getAllByRole<HTMLInputElement>("radio") : [];
}

/** Every choice in an open sheet across all day chips, leaving the opening day active. */
export async function sheetChoices(user: UserEvent, dialog: HTMLElement): Promise<MakeUpChoice[]> {
  const chips = dayChips(dialog);
  if (chips.length === 0) return visibleChoices(dialog);
  const opening = chips.find((chip) => chip.checked);
  const seen = new Map<string, MakeUpChoice>();
  for (const chip of chips) {
    await user.click(chip);
    for (const choice of visibleChoices(dialog)) seen.set(choice.value, choice);
  }
  if (opening) await user.click(opening);
  return [...seen.values()];
}

export async function closeMakeUpSheet(user: UserEvent, dialog: HTMLElement) {
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
}

/** Opens the picker, reads all of its choices, and closes it without changing anything. */
export async function readMakeUpChoices(user: UserEvent, index = 0): Promise<MakeUpChoice[]> {
  const dialog = await openMakeUpSheet(user, index);
  const choices = await sheetChoices(user, dialog);
  await closeMakeUpSheet(user, dialog);
  return choices;
}

/** The committed value: the sheet opens with the saved choice checked. */
export async function makeUpValue(user: UserEvent, index = 0): Promise<string> {
  const dialog = await openMakeUpSheet(user, index);
  const checked = visibleChoices(dialog).find((choice) => choice.checked)?.value ?? "";
  await closeMakeUpSheet(user, dialog);
  return checked;
}

export async function chooseMakeUp(user: UserEvent, value: string, index = 0) {
  const dialog = await openMakeUpSheet(user, index);
  const find = () => [...dialog.querySelectorAll<HTMLInputElement>('fieldset input[type="radio"]')].find((input) => input.value === value) ?? null;
  let radio = find();
  for (const chip of dayChips(dialog)) {
    if (radio) break;
    await user.click(chip);
    radio = find();
  }
  if (!radio) throw new Error(`Make-up choice ${value} is not offered`);
  await user.click(radio);
  await user.click(within(dialog).getByRole("button", { name: "Confirm make-up class" }));
}

/** Waits for a choice on the sheet's current day (e.g. after "See other times"). */
export async function findSheetChoice(dialog: HTMLElement, name: RegExp): Promise<MakeUpChoice> {
  let found: MakeUpChoice | undefined;
  await waitFor(() => {
    found = visibleChoices(dialog).find((choice) => name.test(choice.name));
    if (!found) throw new Error(`No make-up choice matching ${name}`);
  });
  return found!;
}

export const hasChoice = (choices: MakeUpChoice[], name: RegExp) => choices.some((choice) => name.test(choice.name));
