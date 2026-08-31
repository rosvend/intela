package aplicacion

import "slices"

// SoloPropiasObras dice si este usuario puede ver una obra dados los
// titulares que figuran en su declaracion.
//
// OE-6: "El titular solo puede ver los ingresos correspondientes a las obras
// donde tiene participacion registrada." El recorte es de titular; el resto
// de roles ve el catalogo entero: el administrador opera el pipeline, el
// auditor tiene lectura de todo, Distribucion y Contabilidad firman sobre
// el repertorio completo (ADR 0008).
//
// Es un predicado, no un filtro SQL. Los endpoints de datos que aterrizen
// lo aplican para recortar la consulta o para rechazar un GET por id. Se
// define ahora para que no se invente uno distinto en cada handler.
//
// Compara TitularID, no Usuario.ID: son identificadores de agregados
// distintos (usr-ana no es tit-ana) y mezclarlos dejaria al titular fuera
// de sus propias obras.
func SoloPropiasObras(usuario Usuario, titularesDeLaObra []string) bool {
	if usuario.Rol != RolTitular {
		return true
	}
	if usuario.TitularID == "" {
		return false
	}
	return slices.Contains(titularesDeLaObra, usuario.TitularID)
}
