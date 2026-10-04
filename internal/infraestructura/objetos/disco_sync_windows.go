//go:build windows

package objetos

// sincronizarDir no hace nada en Windows. Un fsync de directorio es
// FlushFileBuffers sobre un handle abierto en solo lectura, y Windows lo
// rechaza con "Access is denied" cuando el objeto ya quedo enlazado: Poner
// devolvia error aunque habia escrito, y el reintento chocaba con
// ErrObjetoYaExiste. NTFS registra los cambios de metadatos de directorio en su
// diario, y Disco es el adaptador de DESARROLLO (la produccion usa S3, #182),
// asi que el hueco de durabilidad es aceptable y esta documentado en Poner.
func sincronizarDir(string) error { return nil }
