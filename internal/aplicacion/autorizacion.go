package aplicacion

import "slices"

// SoloPropiasObras dice si este usuario puede ver una obra dados los
// titulares que figuran en su declaracion.
//
// OE-6: "El titular solo puede ver los ingresos correspondientes a las obras
// donde tiene participacion registrada." El recorte es de titular; los
// cuatro roles privilegiados ven el catalogo entero: el administrador opera
// el pipeline, el auditor tiene lectura de todo, Distribucion y Contabilidad
// firman sobre el repertorio completo (ADR 0008).
//
// Falla cerrado, igual que requiereRol: el Usuario cero (Rol vacio) y
// cualquier rol que no este en la matriz no ven nada. Negar solo a
// RolTitular dejaria pasar una peticion sin sesion —o un handler que ignore
// el bool de UsuarioDe— y serviria el catalogo entero.
//
// Es un predicado, no un filtro SQL. Los endpoints de datos que aterrizen
// lo aplican para recortar la consulta o para rechazar un GET por id. Se
// define ahora para que no se invente uno distinto en cada handler.
//
// Compara TitularID, no Usuario.ID: son identificadores de agregados
// distintos (usr-ana no es tit-ana) y mezclarlos dejaria al titular fuera
// de sus propias obras.
func SoloPropiasObras(usuario Usuario, titularesDeLaObra []string) bool {
	switch usuario.Rol {
	case RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor:
		return true
	case RolTitular:
		// TitularID vacio no puede colarse por una cadena vacia de la lista.
		return usuario.TitularID != "" && slices.Contains(titularesDeLaObra, usuario.TitularID)
	default:
		// Usuario cero o rol desconocido: no se ve nada. Igual que
		// requiereRol, este predicado falla cerrado.
		return false
	}
}
