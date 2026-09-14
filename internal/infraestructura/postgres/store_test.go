package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// El limite de transaccion lo fija quien llama, en aplicacion. EnTransaccion
// existe para que los casos de uso que escriben (#18 ingesta, #23
// declaraciones, RepositorioResultados.Guardar, que es "transaccional por
// contrato") tengan la forma desde el primer dia.
func TestEnTransaccionConfirmaAlTerminarBien(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	err := s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo)
			 VALUES ('obra-nueva', 'Recien Declarada', 'Drama', 2020, 'unitario')`)
		return err
	})
	if err != nil {
		t.Fatalf("EnTransaccion: %v", err)
	}

	if _, err := s.ObraPorID(ctx, "obra-nueva"); err != nil {
		t.Fatalf("lo confirmado dentro de la transaccion no se ve fuera: %v", err)
	}
}

// Un resultado a medias es una cifra que alguien puede leer y pagar. Si fn
// falla, no queda nada.
func TestEnTransaccionRevierteSiFnFalla(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	fallo := errors.New("el caso de uso decidio abortar")
	err := s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo)
			 VALUES ('obra-fantasma', 'No Deberia Existir', 'Drama', 2020, 'serie')`); err != nil {
			return err
		}
		return fallo
	})

	// El error de fn sube tal cual: quien llama decide que hacer con el.
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error de fn, se obtuvo %v", err)
	}

	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM obras WHERE id = 'obra-fantasma'`).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Fatal("la transaccion no revirtio: la obra quedo escrita")
	}
}
