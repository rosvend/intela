package aplicacion

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// claveSelloPorDefecto liga la propuesta que se mostro con la que se mide al
// confirmar. No abre sesion ni mueve dinero: solo impide que el cliente
// invente la propuesta contra la que se cuenta la aceptacion. Listar y
// resolver la comparten cuando nadie inyecto otra, para que el sello de la
// bandeja verifique en la confirmacion sin un secreto de despliegue.
var claveSelloPorDefecto = []byte("intela-sello-sugerencia-identificacion")

// propuestaSellada es el contenido del sello: lo justo para reconstruir la
// sugerencia que se mostro, atada al uso. El titulo de catalogo no va: se
// vuelve a leer de las candidatas al pintar.
type propuestaSellada struct {
	UsoID     string   `json:"uso_id"`
	Decision  string   `json:"decision"`
	ObraID    string   `json:"obra_id"`
	Confianza string   `json:"confianza"`
	Motivo    string   `json:"motivo"`
	Orden     []string `json:"orden"`
}

func claveDeSello(clave []byte) []byte {
	if len(clave) == 0 {
		return claveSelloPorDefecto
	}
	return clave
}

// sellarPropuesta devuelve el sello de la propuesta que se va a mostrar para
// ese uso. Un sello de otro uso, o uno alterado, no abre.
func sellarPropuesta(clave []byte, usoID string, s identificacion.Sugerencia) string {
	orden := s.Orden
	if orden == nil {
		orden = []string{}
	}
	bruto, err := json.Marshal(propuestaSellada{
		UsoID:     usoID,
		Decision:  s.Decision,
		ObraID:    s.ObraID,
		Confianza: s.Confianza.String(),
		Motivo:    s.Motivo,
		Orden:     orden,
	})
	if err != nil {
		return ""
	}
	mac := hmac.New(sha256.New, claveDeSello(clave))
	mac.Write(bruto)
	return base64.RawURLEncoding.EncodeToString(bruto) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// abrirPropuesta reconstruye la propuesta que se sello para ese uso. Falla si
// el sello no lo emitio esta clave, o si es de otro caso.
func abrirPropuesta(clave []byte, usoID, sello string) (identificacion.Sugerencia, error) {
	cuerpo, firma, ok := strings.Cut(sello, ".")
	if !ok || cuerpo == "" || firma == "" {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	bruto, err := base64.RawURLEncoding.DecodeString(cuerpo)
	if err != nil {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	suma, err := base64.RawURLEncoding.DecodeString(firma)
	if err != nil {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	mac := hmac.New(sha256.New, claveDeSello(clave))
	mac.Write(bruto)
	if !hmac.Equal(suma, mac.Sum(nil)) {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	var p propuestaSellada
	if err := json.Unmarshal(bruto, &p); err != nil || p.UsoID != usoID {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	confianza, err := decimal.NewFromString(p.Confianza)
	if err != nil {
		return identificacion.Sugerencia{}, errSelloPropuesta
	}
	if p.Orden == nil {
		p.Orden = []string{}
	}
	return identificacion.Sugerencia{
		Decision:  p.Decision,
		ObraID:    p.ObraID,
		Confianza: confianza,
		Motivo:    p.Motivo,
		Orden:     p.Orden,
	}, nil
}

var errSelloPropuesta = errors.New("sello de propuesta invalido")
