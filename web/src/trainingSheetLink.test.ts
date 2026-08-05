import { describe, expect, it } from "vitest";
import { trainingSheetSourceURL } from "./trainingSheetLink";

describe("training sheet source links", () => {
  it("accepts credential-free HTTPS links", () => {
    expect(trainingSheetSourceURL(" https://docs.google.com/spreadsheets/d/workbook/edit#gid=123 ")).toBe(
      "https://docs.google.com/spreadsheets/d/workbook/edit#gid=123"
    );
  });

  it.each([
    undefined,
    "",
    "/spreadsheets/d/workbook",
    "http://docs.google.com/spreadsheets/d/workbook/edit",
    "javascript:alert(1)",
    "https://user:secret@docs.google.com/spreadsheets/d/workbook/edit"
  ])("rejects unsafe or incomplete destinations: %s", (value) => {
    expect(trainingSheetSourceURL(value)).toBeUndefined();
  });
});
