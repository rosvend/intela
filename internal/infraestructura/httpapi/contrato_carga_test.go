package httpapi

import (
	"strings"
	"testing"
)

// El contrato declara de donde sale el actor de una entrega (#116). Quien lee el
// contrato para armar un cliente decide con esta frase si puede mandar el nombre
// de quien sube en el formulario, y la respuesta tiene que ser que no: el actor
// es el de la sesion.
//
// Se comprueba sobre el TEXTO del contrato y no sobre el schema generado porque
// es la prosa la que toma la decision: `subido_por` existe o no existe se ve en
// el JSON, pero "el multipart se ignora" solo esta escrito aqui.
func TestElContratoDiceQueElActorDeUnaEntregaSaleDeLaSesion(t *testing.T) {
	carga := bloqueDelContrato(t, lineasDelContrato(t), "    Carga:")
	subidoPor := bloqueDelContrato(t, carga, "        subido_por:")

	texto := strings.Join(strings.Fields(strings.Join(subidoPor, " ")), " ")
	for _, esperado := range []string{"SESION", "multipart se ignora", "anteriores a la atribucion"} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("la descripcion de Carga.subido_por no dice %q: %q", esperado, texto)
		}
	}
}
