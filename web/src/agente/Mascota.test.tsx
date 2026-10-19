import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Mascota from "./Mascota";

describe("Mascota", () => {
  afterEach(cleanup);

  it("es decorativa: la oculta a los lectores y dibuja dos ojos", () => {
    const { container } = render(<Mascota />);
    const svg = container.querySelector("svg.mascota");
    expect(svg?.getAttribute("aria-hidden")).toBe("true");
    expect(svg?.querySelectorAll(".mascota-ojo")).toHaveLength(2);
  });
});
