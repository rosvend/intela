//go:build !windows

package objetos

import "os"

// sincronizarDir fuerza a disco la entrada de directorio de lo que se acaba de
// enlazar.
func sincronizarDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
