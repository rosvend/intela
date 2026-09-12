package normalizacion

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ParsearFecha acepta los formatos perfilados en las parrillas reales
// (docs/dominio/fuentes-datos.md): entero YYYYMMDD, ISO, y el serial de
// Excel que es como viaja un "objeto de tiempo" cuando el adaptador no lo
// convirtio. Cualquier otra cosa es inparseable: no se inventa una fecha
// cero, se reporta.
func ParsearFecha(s string) (Fecha, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Fecha{}, nil
	}

	if f, ok := parsearISO(s); ok {
		return f, nil
	}
	if f, ok := parsearYYYYMMDD(s); ok {
		return f, nil
	}
	if f, ok := parsearSerialExcel(s); ok {
		return f, nil
	}
	return Fecha{}, fmt.Errorf("fecha %q: no es YYYYMMDD, ISO ni serial de Excel", s)
}

// ParsearHora acepta HH:MM[:SS] y la fraccion de dia de Excel (0.5 = 12:00).
// Vacio es hora desconocida, no un error: la parrilla a veces trae Fecha
// sin Hora, y al reves.
func ParsearHora(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}

	if h, ok := parsearReloj(s); ok {
		return h, nil
	}
	if h, ok := parsearFraccionDia(s); ok {
		return h, nil
	}
	return "", fmt.Errorf("hora %q: no es HH:MM[:SS] ni fraccion de dia", s)
}

func parsearISO(s string) (Fecha, bool) {
	// YYYY-MM-DD o YYYY/MM/DD, con o sin cola de tiempo (T20:00:00).
	s = strings.ReplaceAll(s, "/", "-")
	if i := strings.IndexAny(s, "T "); i >= 0 {
		s = s[:i]
	}
	partes := strings.Split(s, "-")
	if len(partes) != 3 {
		return Fecha{}, false
	}
	anio, errA := strconv.Atoi(partes[0])
	mes, errM := strconv.Atoi(partes[1])
	dia, errD := strconv.Atoi(partes[2])
	if errA != nil || errM != nil || errD != nil {
		return Fecha{}, false
	}
	f := Fecha{Anio: anio, Mes: mes, Dia: dia}
	if !f.valida() {
		return Fecha{}, false
	}
	return f, true
}

func parsearYYYYMMDD(s string) (Fecha, bool) {
	s = recortarFraccion(s)
	if len(s) != 8 || !soloDigitos(s) {
		return Fecha{}, false
	}
	anio, _ := strconv.Atoi(s[:4])
	mes, _ := strconv.Atoi(s[4:6])
	dia, _ := strconv.Atoi(s[6:8])
	f := Fecha{Anio: anio, Mes: mes, Dia: dia}
	if !f.valida() {
		return Fecha{}, false
	}
	return f, true
}

// parsearSerialExcel convierte el numero de serie de Excel (Windows).
//
// El serial 25569 es el 1970-01-01; a partir de ahi se cuenta en dias
// civiles. El serial 60 es el 29 de febrero de 1900, que Excel invento y
// el calendario real no tiene: se rechaza, no se "arregla" a marzo.
func parsearSerialExcel(s string) (Fecha, bool) {
	s = strings.TrimSpace(s)
	entero, _, ok := partirNumero(s)
	if !ok {
		return Fecha{}, false
	}
	// YYYYMMDD son 8 digitos y ya se intentaron. Un serial de 2024 ronda
	// los 45000. Por encima de 100000 no es una fecha de parrilla.
	if entero < 1 || entero > 100000 {
		return Fecha{}, false
	}
	if entero == 60 {
		return Fecha{}, false
	}
	return diasDesdeEpochExcel(entero), true
}

// 25569 es el serial de Excel del 1970-01-01 (epoch Unix en dias). Contar
// desde ahi evita arrastrar el 29 de febrero de 1900 en las fechas que
// realmente llegan (2024-2026).
const serialExcelUnix = 25569

func diasDesdeEpochExcel(serial int) Fecha {
	return Fecha{Anio: 1970, Mes: 1, Dia: 1}.masDias(serial - serialExcelUnix)
}

func parsearReloj(s string) (string, bool) {
	partes := strings.Split(s, ":")
	if len(partes) != 2 && len(partes) != 3 {
		return "", false
	}
	h, errH := strconv.Atoi(partes[0])
	m, errM := strconv.Atoi(partes[1])
	seg := 0
	var errS error
	if len(partes) == 3 {
		seg, errS = strconv.Atoi(partes[2])
	}
	if errH != nil || errM != nil || errS != nil {
		return "", false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 || seg < 0 || seg > 59 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, seg), true
}

func parsearFraccionDia(s string) (string, bool) {
	if strings.Contains(s, ":") {
		return "", false
	}
	frac, err := strconv.ParseFloat(s, 64)
	if err != nil || frac < 0 || frac >= 1 {
		return "", false
	}
	// 86400 segundos al dia. Se redondea al segundo mas cercano: una
	// fraccion binaria de Excel nunca cae justo en un segundo.
	total := int(frac*86400 + 0.5)
	if total >= 86400 {
		total = 86399
	}
	h := total / 3600
	m := (total % 3600) / 60
	seg := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, seg), true
}

func (f Fecha) valida() bool {
	if f.Anio < 1900 || f.Anio > 2100 {
		return false
	}
	if f.Mes < 1 || f.Mes > 12 {
		return false
	}
	return f.Dia >= 1 && f.Dia <= diasDelMes(f.Anio, f.Mes)
}

func diasDelMes(anio, mes int) int {
	dias := [...]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if mes == 2 && esBisiesto(anio) {
		return 29
	}
	return dias[mes]
}

func esBisiesto(anio int) bool {
	return anio%4 == 0 && (anio%100 != 0 || anio%400 == 0)
}

func (f Fecha) masDias(dias int) Fecha {
	if dias >= 0 {
		f.Dia += dias
		for {
			dim := diasDelMes(f.Anio, f.Mes)
			if f.Dia <= dim {
				return f
			}
			f.Dia -= dim
			f.Mes++
			if f.Mes > 12 {
				f.Mes = 1
				f.Anio++
			}
		}
	}
	f.Dia += dias
	for f.Dia < 1 {
		f.Mes--
		if f.Mes < 1 {
			f.Mes = 12
			f.Anio--
		}
		f.Dia += diasDelMes(f.Anio, f.Mes)
	}
	return f
}

func recortarFraccion(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i]
	}
	return s
}

func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func partirNumero(s string) (entero int, frac float64, ok bool) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, 0, false
	}
	entero = int(n)
	if n < 0 || float64(entero) > n+0.0000001 {
		return 0, 0, false
	}
	return entero, n - float64(entero), true
}
