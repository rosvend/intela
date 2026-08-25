// Package migrations embebe los ficheros .sql en el binario.
//
// Embebidas y no montadas: asi `migrate` no depende de que el despliegue
// monte el directorio correcto, y la version del esquema viaja con la version
// del codigo que la espera.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
