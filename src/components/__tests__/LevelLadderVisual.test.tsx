import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { LevelLadderVisual } from "../LevelLadderVisual";

it("renders level ladder with all 4 levels", () => {
  render(<LevelLadderVisual minLevelForSitLower={2} />);
  expect(screen.getByText("Level 1")).toBeInTheDocument();
  expect(screen.getByText("Level 2")).toBeInTheDocument();
  expect(screen.getByText("Level 3")).toBeInTheDocument();
  expect(screen.getByText("Level 4")).toBeInTheDocument();
});

it("shows Zoom badge for level 1 always", () => {
  render(<LevelLadderVisual minLevelForSitLower={2} />);
  expect(screen.getByText("Zoom")).toBeInTheDocument();
});

it("shows sit-in direction arrows for non-top levels", () => {
  render(<LevelLadderVisual minLevelForSitLower={2} />);
  expect(screen.getByText(/sits in Level 3$/)).toBeInTheDocument();
  expect(screen.getByText(/sits in Level 4/)).toBeInTheDocument();
});

it("shows sit-in lower direction for top level", () => {
  render(<LevelLadderVisual minLevelForSitLower={2} />);
  expect(screen.getByText(/sits in Level 3.*lower/)).toBeInTheDocument();
});

it("renders with custom min level", () => {
  render(<LevelLadderVisual minLevelForSitLower={3} />);
  expect(screen.getByText("Level 1")).toBeInTheDocument();
  expect(screen.getByText("Level 2")).toBeInTheDocument();
  expect(screen.getByText("Level 3")).toBeInTheDocument();
  expect(screen.getByText("Level 4")).toBeInTheDocument();
});

it("shows no sit-in for levels below minLevelForSitLower", () => {
  render(<LevelLadderVisual minLevelForSitLower={3} />);
  expect(screen.getByText("no sit-in")).toBeInTheDocument();
  expect(screen.getByText(/sits in Level 4/)).toBeInTheDocument();
});

it("top level always sits lower when minLevelForSitLower allows it", () => {
  render(<LevelLadderVisual minLevelForSitLower={3} />);
  expect(screen.getByText(/sits in Level 3.*lower/)).toBeInTheDocument();
});
