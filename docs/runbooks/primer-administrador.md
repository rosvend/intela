# Runbook: provisionar el primer administrador

Crea la cuenta inicial de una instalacion **vacia**. Una vez hecho, no se puede repetir.

Por que existe y por que es asi: [ADR 0017](../decisiones/0017-provision-del-primer-administrador.md).

## Cuando se usa

Una sola vez por instalacion, cuando `usuarios` esta vacia y por eso nadie puede entrar. Sintoma:
`/api/health` y `/api/ready` responden 200 y cualquier otra ruta responde 401.

**No sirve para nada mas.** No crea los otros roles, no resetea claves y no toca una instalacion que
ya tenga algun usuario -- ni siquiera si solo tiene titulares.

## Antes de empezar

- Credenciales AWS del entorno y permiso `lambda:InvokeFunction` sobre `intela-migrate`.
- El repositorio y Go, para calcular el hash. No hace falta acceso a la base.

## Pasos

### 1. Comprobar que la instalacion esta cerrada

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://<url>/api/ready   # 200
curl -s -o /dev/null -w '%{http_code}\n' https://<url>/api/obras   # 401
```

Un 200 en `/api/obras` sin cabecera de sesion seria otro problema, no este.

### 2. Calcular el hash de la clave

```bash
go run ./cmd/hash-clave > /tmp/hash.txt
# escribe la clave, Enter
```

La clave se teclea, **no se pasa como argumento**: un argumento es visible en `ps` para toda la
maquina y queda en `~/.bash_history`. El aviso sale por stderr y el hash por stdout, asi que el
fichero contiene el hash y nada mas.

Se usa este comando y no un script equivalente en otro lenguaje porque usa el mismo `cripto.Bcrypt`
que despues verifica el login: mismo algoritmo y mismo coste por construccion.

### 3. Invocar la provision

```bash
hash=$(cat /tmp/hash.txt)
aws lambda invoke \
  --function-name intela-migrate \
  --profile <perfil> \
  --payload "$(jq -nc --arg h "$hash" '{
      orden:  "primer-administrador",
      id:     "usr-admin",
      email:  "admin@redes.co",
      nombre: "Administrador",
      hash:   $h
  }')" \
  --cli-binary-format raw-in-base64-out \
  /dev/stdout
```

`jq -nc --arg` en vez de interpolar el hash en una cadena: un hash bcrypt lleva `$` y `/`, y armar
el JSON a mano lo rompe de formas que no siempre fallan de golpe.

Respuesta esperada:

```json
{"orden":"primer-administrador","estado":"creado"}
```

### 4. Borrar el hash y entrar

```bash
rm -f /tmp/hash.txt
```

Inicia sesion en la interfaz con el email y la clave. **Si el login falla, ver "Si algo va mal".**

## Respuestas posibles

| Respuesta | Que significa | Que hacer |
| --- | --- | --- |
| `{"estado":"creado"}` | La cuenta existe. | Entrar y borrar el hash. |
| `{"estado":"ya provisionada"}` | Ya habia usuarios. No se creo ni se toco nada. | Nada. Si nadie puede entrar, ver abajo. |
| `usuario invalido: ... no tiene forma de hash` | Se mando la clave en claro, o el JSON se rompio con el `$` del hash. | Repetir el paso 2 con `jq`. **No se escribio nada.** |
| `usuario invalido: falta el <campo>` | Falta un campo del evento. | Completarlo. No se abrio ni la conexion. |

Un reintento es inocuo: la operacion es de un solo uso y la segunda invocacion no hace nada.
Tambien lo es el reintento automatico de una invocacion asincrona.

## Si algo va mal

**La cuenta se creo pero el login falla.** La validacion cubre lo que se ha visto fallar hasta hoy
-- una clave en claro, un hash truncado, uno con caracteres de sobra -- y hay una prueba que
provisiona y despues inicia sesion de verdad. Pero la lista de formas de estropear un hash ya
crecio dos veces durante la revision de este mismo cambio, asi que **no se afirma que sea
imposible**. Si pasa, no hay via de arreglo desde la aplicacion: los unicos escritores de `usuarios` son esta orden, que
ya no vuelve a correr, y `cmd/seed`, que no esta en la imagen de produccion. Hace falta acceso a la
base desde dentro de la VPC. **Escribirlo como incidente y abrir un issue de gestion de usuarios**,
que es la pieza que falta.

**Nadie puede entrar y la respuesta es `ya provisionada`.** Hay una cuenta cuyo credencial nadie
conoce. Mismo camino que el anterior: hoy no hay reset de clave.

## Nota de seguridad

Quien tenga `lambda:InvokeFunction` sobre `intela-migrate` puede reclamar el primer administrador
antes que el operador. La ventana se cierra en la primera invocacion, asi que conviene hacer este
procedimiento **pronto** tras el primer despliegue de un entorno, y revisar quien tiene ese permiso
(issues #92 y #94). No es una vulnerabilidad nueva de esta orden: quien puede invocar esa funcion
ya podia aplicar migraciones.

## Lo que este runbook NO cubre

Crear los otros cuatro roles del reglamento, y dar credenciales a un titular aprobado por la
afiliacion (#50). Ninguna de las dos tiene hoy un camino en el producto; ver las consecuencias del
[ADR 0017](../decisiones/0017-provision-del-primer-administrador.md).
