import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import CrossStudyCrmRowCard from "../CrossStudyCrmRowCard";

describe("CrossStudyCrmRowCard", () => {
	it("shows the merge group and paired course for a merged CRM source", () => {
		// Given: a CRM source row resolved to a merge-group member.
		render(
			<CrossStudyCrmRowCard
				crmRow={{
					snapshot_id: "snapshot-id",
					row_hash: "row-hash",
					xlsx_row_number: 7,
					course_name: "Merged Source A",
					course_id: "course-a-id",
					extra_note: "",
					imported_at: "2026-08-26T00:00:00Z",
					merge_group_id: "merge-group-id",
					merge_group_name: "Source Merge Group",
					merge_group_peer_course_id: "course-b-id",
					merge_group_peer_course_name: "Merged Source B",
				}}
				selected={false}
				onSelect={vi.fn()}
			/>,
		);

		// When: the CRM row card is presented to staff.
		const mergeContext = screen.getByLabelText(/merge group context/i);

		// Then: the group name and paired course are both visible.
		expect(mergeContext).toHaveTextContent("Source Merge Group");
		expect(mergeContext).toHaveTextContent("Merged Source B");
	});
});
