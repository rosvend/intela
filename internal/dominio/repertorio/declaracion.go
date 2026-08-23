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
