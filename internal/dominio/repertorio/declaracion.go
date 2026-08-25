package repertorio

import "github.com/shopspring/decimal"

type Parte struct {
	TitularID  string
	IPI        string
	Porcentaje decimal.Decimal
}

type Declaracion struct {
	ObraID string
	Partes []Parte
}

func (d Declaracion) Completa() bool {
	if len(d.Partes) == 0 {
		return false
	}
	suma := decimal.Zero
	for _, p := range d.Partes {
		if p.IPI == "" {
			return false
		}
		// Cada parte tiene que ser positiva por si sola. Sin esto, 150 y -50
		// suman 100 y una declaracion imposible pasa por "completa", que es
		// justo la puerta que R-04 (RD 13.1.3) cierra: si lo declarado no
		// suma 100%, no se reparte nada de esa obra.
		if p.Porcentaje.LessThanOrEqual(decimal.Zero) {
			return false
		}
		suma = suma.Add(p.Porcentaje)
	}
	return suma.Equal(decimal.NewFromInt(100))
}

func (d Declaracion) Estado() string {
	if d.Completa() {
		return "completa"
	}
	return "incompleta"
}
