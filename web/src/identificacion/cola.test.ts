import { describe, expect, it } from "vitest";
import { posicionEnCola, vecinoEnCola } from "./cola";

const ids = ["a", "b", "c", "d"];

describe("vecinoEnCola", () => {
  it("salta al siguiente o al anterior que sigue en la cola", () => {
    const visibles = new Set(["a", "b", "c", "d"]);
    expect(vecinoEnCola(ids, visibles, "b", 1)).toBe("c");
    expect(vecinoEnCola(ids, visibles, "b", -1)).toBe("a");
  });

  it("se salta los que ya salieron, incluido el propio", () => {
    const visibles = new Set(["a", "d"]);
    expect(vecinoEnCola(ids, visibles, "b", 1)).toBe("d");
    expect(vecinoEnCola(ids, visibles, "c", -1)).toBe("a");
  });

  it("en un extremo no hay vecino", () => {
    const visibles = new Set(ids);
    expect(vecinoEnCola(ids, visibles, "d", 1)).toBeNull();
    expect(vecinoEnCola(ids, visibles, "a", -1)).toBeNull();
    expect(vecinoEnCola(ids, visibles, "x", 1)).toBeNull();
  });
});

describe("posicionEnCola", () => {
  it("cuenta desde 1 entre los visibles; null si el caso ya salio", () => {
    const visibles = new Set(["a", "c", "d"]);
    expect(posicionEnCola(ids, visibles, "c")).toBe(2);
    expect(posicionEnCola(ids, visibles, "b")).toBeNull();
  });
});
