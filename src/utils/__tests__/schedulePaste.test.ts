import { describe, expect, it } from "vitest";
import { parseSchedulePaste } from "../schedulePaste";

describe("parseSchedulePaste", () => {
  it("parses Warwick schedule TSV rows with a header", () => {
    const input = [
      "Date\tBegin\tEnd\tDuration\tClassroom\tConfirm\tBy\t",
      "Sun 31 May 26\t13:00\t15:00\t02:00\t\t\t\t",
      "Sat 06 Jun 26\t15:00\t16:30\t01:30\t\t\t\t",
      "Sun 16 Aug 26\t13:00\t15:00\t02:00",
    ].join("\n");

    expect(parseSchedulePaste(input)).toEqual({
      rows: [
        {
          rowNumber: 2,
          date: "2026-05-31",
          begin: "13:00",
          end: "15:00",
          duration: "02:00",
          classroom: "",
        },
        {
          rowNumber: 3,
          date: "2026-06-06",
          begin: "15:00",
          end: "16:30",
          duration: "01:30",
          classroom: "",
        },
        {
          rowNumber: 4,
          date: "2026-08-16",
          begin: "13:00",
          end: "15:00",
          duration: "02:00",
          classroom: "",
        },
      ],
      errors: [],
    });
  });

  it("reports row-level errors without dropping valid rows", () => {
    expect(parseSchedulePaste("Date\tBegin\tEnd\nBad date\t13:00\t15:00\nSun 31 May 26\t13:00\t15:00")).toEqual({
      rows: [
        {
          rowNumber: 3,
          date: "2026-05-31",
          begin: "13:00",
          end: "15:00",
          duration: "",
          classroom: "",
        },
      ],
      errors: [{ rowNumber: 2, message: "Invalid date" }],
    });
  });
});
