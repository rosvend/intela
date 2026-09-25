package aplicacion

import (
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// anomalias.ModalidadPonderaPorTipoObra copia en strings las modalidades de reparto: esta prueba
// falla si reparto gana o pierde una sin que anomalias la clasifique (revision de #158, punto 9).
func TestLasModalidadesDeTipoObraSonLasDeReparto(t *testing.T) {
	var deReparto []string
	for _, m := range reparto.Modalidades() {
		parseada, err := reparto.ParseModalidad(string(m))
		if err != nil {
			t.Fatalf("reparto.Modalidades() devuelve %q y ParseModalidad lo rechaza: %v", m, err)
		}
		deReparto = append(deReparto, string(parseada))
	}
	slices.Sort(deReparto)

	clasificadas := anomalias.ModalidadesClasificadas()
	if !slices.Equal(deReparto, clasificadas) {
		t.Fatalf("modalidades de reparto %v != clasificadas en anomalias.ModalidadPonderaPorTipoObra %v", deReparto, clasificadas)
	}
}
