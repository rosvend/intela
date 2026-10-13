// Package herramientas define las herramientas de solo lectura del agente, una por archivo.
//
// Cada constructor devuelve una aplicacion.Herramienta que envuelve un caso de uso de lectura de
// aplicacion y le pasa el actor de la sesion: el RBAC vive en ese caso de uso, nunca aqui. Este
// paquete no importa internal/dominio (regla depguard herramientas-sin-dominio) ni expone casos
// de uso de escritura. La forma (Nombre, Descripcion, Esquema, Ejecutar) es la de una tool de MCP.
//
// Se registran en internal/infraestructura/asistente/herramientas.go, una linea por herramienta.
package herramientas
