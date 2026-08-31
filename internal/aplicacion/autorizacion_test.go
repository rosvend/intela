package aplicacion

import "testing"

func TestSoloPropiasObrasTitularConParticipacionVeLaObra(t *testing.T) {
	u := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
	if !SoloPropiasObras(u, []string{"tit-otro", "tit-ana"}) {
		t.Fatal("el titular tiene que ver una obra donde figura")
	}
}

func TestSoloPropiasObrasTitularSinParticipacionNoVeLaObra(t *testing.T) {
	u := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
	if SoloPropiasObras(u, []string{"tit-otro"}) {
		t.Fatal("el titular no puede ver una obra ajena")
	}
}

func TestSoloPropiasObrasTitularSinTitularIDNoVeNada(t *testing.T) {
	u := Usuario{ID: "usr-ana", Rol: RolTitular}
	if SoloPropiasObras(u, []string{"", "tit-ana"}) {
		t.Fatal("un titular sin TitularID no puede colarse por una cadena vacia")
	}
}

// Usuario.ID y TitularID son agregados distintos. Si el predicado comparara
// el primero, un titular quedaria fuera de sus propias obras —o, peor,
// dentro de las de otro cuyo id de usuario coincidiera.
func TestSoloPropiasObrasComparaTitularIDNoUsuarioID(t *testing.T) {
	u := Usuario{ID: "tit-ana", Rol: RolTitular, TitularID: "tit-ana-real"}
	if SoloPropiasObras(u, []string{"tit-ana"}) {
		t.Fatal("se compara TitularID, no el ID de usuario")
	}
}

func TestSoloPropiasObrasNoRecortaAlPersonal(t *testing.T) {
	titulares := []string{"tit-ana"}
	for _, rol := range []Rol{RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor} {
		u := Usuario{ID: "usr-1", Rol: rol}
		if !SoloPropiasObras(u, titulares) {
			t.Fatalf("%s tiene que ver cualquier obra: el recorte es de titular (OE-6)", rol)
		}
	}
}
