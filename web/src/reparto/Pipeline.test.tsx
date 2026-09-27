import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import Pipeline from "./Pipeline";
import {
  ETAPAS_INTERNACIONAL,
  ETAPAS_NACIONAL,
  ETIQUETA_ETAPA,
} from "./etapas";
import { Etapa } from "./tipos";

describe("Pipeline", () => {
  afterEach(() => {
    cleanup();
  });

  it.each(ETAPAS_NACIONAL)(
    "marca %s como paso actual en el circuito nacional",
    (etapa: Etapa) => {
      render(<Pipeline circuito="nacional" etapa={etapa} />);
      const actual = screen.getByRole("listitem", { current: "step" });
      expect(actual.textContent).toBe(ETIQUETA_ETAPA[etapa]);
      expect(screen.getByText("Importe de la obra")).toBeTruthy();
      expect(screen.queryByText("Fees in Error")).toBeNull();
      cleanup();
    },
  );

  it.each(ETAPAS_INTERNACIONAL)(
    "marca %s como paso actual en el circuito internacional",
    (etapa: Etapa) => {
      render(<Pipeline circuito="internacional" etapa={etapa} />);
      const actual = screen.getByRole("listitem", { current: "step" });
      expect(actual.textContent).toBe(ETIQUETA_ETAPA[etapa]);
      expect(screen.queryByText("Importe de la obra")).toBeNull();
      expect(screen.getByText("Fees in Error")).toBeTruthy();
      cleanup();
    },
  );
});
