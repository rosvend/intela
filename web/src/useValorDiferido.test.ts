import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEBOUNCE_TECLEO_MS, useValorDiferido } from "./useValorDiferido";

describe("useValorDiferido", () => {
  beforeEach(() => {
    // El cronometro es del test: con temporizadores de verdad, "todavia no
    // cambio" y "ya cambio" no se pueden separar sin dormir la prueba.
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("el primer valor sale sin esperar", () => {
    const { result } = renderHook(() =>
      useValorDiferido("C", DEBOUNCE_TECLEO_MS),
    );

    expect(result.current).toBe("C");
  });

  it("un cambio espera a que pare, y al vencer la espera sale el valor nuevo", async () => {
    const { result, rerender } = renderHook(
      ({ valor }) => useValorDiferido(valor, DEBOUNCE_TECLEO_MS),
      { initialProps: { valor: "C" } },
    );

    await act(async () => {
      rerender({ valor: "Ca" });
    });
    expect(result.current).toBe("C");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS - 1);
    });
    expect(result.current).toBe("C");

    // Control positivo: un hook que esperara para siempre pasaria todo lo de
    // arriba sin servir para nada.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(result.current).toBe("Ca");
  });

  it("una rafaga de diez cambios emite una sola vez", async () => {
    const emitidos: string[] = [];
    const { rerender } = renderHook(
      ({ valor }) => {
        const diferido = useValorDiferido(valor, DEBOUNCE_TECLEO_MS);
        if (emitidos[emitidos.length - 1] !== diferido) emitidos.push(diferido);
        return diferido;
      },
      { initialProps: { valor: "" } },
    );

    for (let i = 1; i <= 10; i++) {
      await act(async () => {
        rerender({ valor: "abcdefghij".slice(0, i) });
        await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS - 50);
      });
    }
    await act(async () => {
      await vi.advanceTimersByTimeAsync(DEBOUNCE_TECLEO_MS);
    });

    expect(emitidos).toEqual(["", "abcdefghij"]);
  });
});
