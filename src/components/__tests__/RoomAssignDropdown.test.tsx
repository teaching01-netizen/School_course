import { useRef, useState } from "react";
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RoomAssignDropdown, { type RoomAssignRoom } from "../RoomAssignDropdown";

const ROOMS: RoomAssignRoom[] = [
  { id: "r1", name: "Room 101", capacity: 20 },
  { id: "r2", name: "Room 102", capacity: null },
  { id: "r3", name: "Studio A", capacity: 8 },
];

/** Mirrors the real parent: the committed value is reflected back via the prop. */
function StatefulHarness({
  initialValue = null as string | null,
  rooms = ROOMS,
  busy,
  disabled = false,
  saving = false,
}: {
  initialValue?: string | null;
  rooms?: RoomAssignRoom[];
  busy?: Map<string, string>;
  disabled?: boolean;
  saving?: boolean;
}) {
  const [value, setValue] = useState<string | null>(initialValue);
  const commitsRef = useRef<Array<string | null>>([]);
  return (
    <div>
      <RoomAssignDropdown
        value={value}
        onCommit={(v) => {
          commitsRef.current.push(v);
          setValue(v);
        }}
        rooms={rooms}
        busy={busy}
        disabled={disabled}
        saving={saving}
      />
      <output data-testid="commits">{commitsRef.current.join(",")}</output>
    </div>
  );
}

describe("RoomAssignDropdown", () => {
  it("renders a trigger with the placeholder when unassigned", () => {
    render(<StatefulHarness />);
    const trigger = screen.getByRole("button", { name: /assign room/i });
    expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("shows the assigned room name on the trigger", () => {
    render(<StatefulHarness initialValue="r1" />);
    expect(screen.getByRole("button", { name: /room 101/i })).toBeInTheDocument();
  });

  it("opens the popover on click and focuses the search input", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    const listbox = screen.getByRole("listbox");
    expect(listbox).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /assign room/i })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("combobox")).toHaveFocus();
  });

  it("lists every room with a checkbox", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    expect(screen.getAllByRole("option")).toHaveLength(3);
    expect(screen.getByRole("checkbox", { name: /room 101/i })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /room 102/i })).not.toBeChecked();
  });

  it("shows capacity as secondary information under each room", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    expect(screen.getByText("20 seats")).toBeInTheDocument();
    expect(screen.getByText("8 seats")).toBeInTheDocument();
  });

  it("filters rooms by search query", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    await user.type(screen.getByRole("combobox"), "101");

    expect(screen.getByRole("option", { name: /room 101/i })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /room 102/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /studio a/i })).not.toBeInTheDocument();
  });

  it("shows an empty state when the search matches nothing", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    await user.type(screen.getByRole("combobox"), "zzz");

    expect(screen.getByText(/no rooms found/i)).toBeInTheDocument();
  });

  it("commits when an unassigned room is checked and keeps the popover open", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    await user.click(screen.getByRole("checkbox", { name: /room 101/i }));

    expect(screen.getByTestId("commits")).toHaveTextContent("r1");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /room 101/i })).toBeChecked();
    expect(screen.getByRole("button", { name: /room 101/i })).toBeInTheDocument();
  });

  it("clears the assignment when the assigned room is unchecked", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness initialValue="r1" />);
    await user.click(screen.getByRole("button", { name: /room 101/i }));

    await user.click(screen.getByRole("checkbox", { name: /room 101/i }));

    expect(screen.getByTestId("commits")).toHaveTextContent("");
    expect(screen.getByRole("checkbox", { name: /room 101/i })).not.toBeChecked();
    expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
  });

  it("moves the assignment when another room is checked", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness initialValue="r1" />);
    await user.click(screen.getByRole("button", { name: /room 101/i }));

    await user.click(screen.getByRole("checkbox", { name: /room 102/i }));

    expect(screen.getByTestId("commits")).toHaveTextContent("r2");
    expect(screen.getByRole("checkbox", { name: /room 102/i })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /room 101/i })).not.toBeChecked();
  });

  it("disables busy rooms without committing and shows the busy reason", async () => {
    const user = userEvent.setup();
    const busy = new Map([["r2", "Busy 13:00–14:00"]]);
    render(<StatefulHarness busy={busy} />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    await user.click(screen.getByRole("checkbox", { name: /room 102/i }));

    expect(screen.getByTestId("commits")).toHaveTextContent("");
    expect(screen.getByRole("option", { name: /room 102/i })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("checkbox", { name: /room 102/i })).toBeDisabled();
    expect(screen.getByText(/busy 13:00–14:00/i)).toBeInTheDocument();
  });

  it("clears via the trigger close button without opening the popover", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness initialValue="r1" />);

    await user.click(screen.getByRole("button", { name: /clear room/i }));

    expect(screen.getByTestId("commits")).toHaveTextContent("");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /assign room/i })).toBeInTheDocument();
  });

  it("traverses options with arrow keys and commits with Enter", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    const input = screen.getByRole("combobox");

    await user.keyboard("{ArrowDown}");
    expect(input).toHaveAttribute("aria-activedescendant", "assign-room-option-r1");
    await user.keyboard("{ArrowDown}");
    await user.keyboard("{ArrowDown}");
    expect(input).toHaveAttribute("aria-activedescendant", "assign-room-option-r3");
    await user.keyboard("{Enter}");

    expect(screen.getByTestId("commits")).toHaveTextContent("r3");
  });

  it("commits the highlighted option with Space", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));

    await user.keyboard("{ArrowDown}");
    await user.keyboard(" ");

    expect(screen.getByTestId("commits")).toHaveTextContent("r1");
  });

  it("closes on Escape and restores focus to the trigger", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /assign room/i })).toHaveFocus();
  });

  it("closes on outside click", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness />);
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    await user.click(document.body);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("does not open when disabled", async () => {
    const user = userEvent.setup();
    render(<StatefulHarness disabled />);

    expect(screen.getByRole("button", { name: /assign room/i })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: /assign room/i }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("marks the trigger busy while saving", () => {
    render(<StatefulHarness saving />);
    expect(screen.getByRole("button", { name: /assign room/i })).toHaveAttribute("aria-busy", "true");
  });
});