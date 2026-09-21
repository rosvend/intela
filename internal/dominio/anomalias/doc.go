// Package anomalias revisa un periodo ENTERO antes de que se reparta y
// nombra lo que impide, o falsea, el reparto de ese periodo (OE-5 / KR-4).
//
// No toca dinero y no decide pagos: dice que filas y que obras no estan en
// condiciones de ponderar. Por eso depguard le deniega reparto, recaudo,
// liquidacion y anticipos, igual que a identificacion y a oni (ADR 0003).
//
// # Por que es un paso aparte y no una validacion de la ingesta
//
// Tres de los seis detectores necesitan el periodo ARMADO, y no se pueden
// contestar mirando un archivo:
//
//   - el mismo sha256 entregado por DOS fuentes distintas no se ve hasta que
//     estan las dos entregas (el UNIQUE (sha256, fuente) solo cierra la
//     repeticion de la MISMA fuente);
//   - dos registros logicamente iguales en DOS archivos del mismo periodo no
//     se ven leyendo uno solo (la comprobacion intra-archivo ya la hace el
//     adaptador de formato al ingerir, ver ingesta.Mapa.ClaveRegistro);
//   - la retencion de `R-04` depende de la declaracion VIGENTE, que puede
//     haber cambiado despues de la ingesta.
//
// # Todo es puro y el recorrido es estable
//
// Ningun detector hace E/S, lee el reloj ni recibe un puerto: la capa de
// aplicacion reune los datos y llama a [Detectar]. Los recorridos van sobre
// listas ordenadas y el resultado se ordena antes de devolverse, porque el
// recorrido de un mapa de Go es aleatorio y el ADR 0005 exige que dos
// pasadas sobre el mismo dato produzcan lo mismo, en el mismo orden.
//
// # Los seis detectores
//
//  1. [TipoONI] -- la cascada corrio y no reconocio la obra (ADR 0007).
//  2. [TipoDuplicadoArchivo] -- los mismos bytes en dos entregas.
//  3. [TipoDuplicadoRegistro] -- el mismo registro logico en dos archivos
//     del mismo periodo (ADR 0018 fija la clave).
//  4. [TipoTitularSinPorcentaje] -- un coautor del catalogo sin parte
//     declarada, o una parte sin IPI.
//  5. [TipoReservaDeclaracionIncompleta] -- `R-04` / `RD 13.1.3`: lo
//     declarado no suma 100 y se retiene el TOTAL de esa obra.
//  6. [TipoTipoObraSinMapear] -- la obra no cae en ninguna de las cuatro
//     categorias de `RD 9.1.1`.
//
// El 4 y el 5 miran el mismo hecho desde dos alturas y NO son el mismo
// aviso: ver el comentario de [titularesSinPorcentaje].
//
// # Critica no quiere decir grave
//
// [EsCritica] separa lo que hace que el periodo se reparta MAL de lo que es
// un estado valido del modelo. Una obra retenida por declaracion incompleta
// no bloquea nada -- el dinero se retiene entero y el reparto sigue --;
// un archivo contado dos veces si, porque falsea el valor punto de todas las
// demas obras del canal.
package anomalias
