# Informe del TP Nivelador

## Arquitectura general

El sistema está compuesto por clientes escritos en Go que representan agencias de
lotería y un servidor escrito en Python que representa la central de Lotería
Nacional. Los procesos se comunican mediante sockets TCP y un protocolo binario
propio.

Cada cliente procesa su archivo de entrada de manera incremental y agrupa las
apuestas en lotes configurables. El servidor crea un proceso por conexión, persiste
las apuestas mediante el modelo de dominio provisto y devuelve únicamente los
ganadores pertenecientes a la agencia que abrió la conexión.

## Protocolo de comunicación

### Convenciones

- Los enteros son sin signo y se codifican en orden de red (big-endian).
- Las longitudes de strings representan bytes UTF-8, no caracteres.
- Todos los mensajes poseen un encabezado fijo y un payload de longitud variable.

### Framing

El encabezado tiene 5 bytes:

- 1 byte para el tipo (identificador del mensaje)
- 4 bytes para la longitud (cantidad de bytes del payload)

La longitud no incluye los 5 bytes del encabezado. El receptor primero obtiene el
encabezado completo, valida el tipo y la longitud, y luego recibe exactamente el
payload declarado.

### Tipos de mensajes

| Valor | Nombre | Dirección | Propósito |
|---:|---|---|---|
| `0x01` | `AGENCY` | Cliente a servidor | Registrar la agencia de la conexión |
| `0x02` | `BETS` | Cliente a servidor | Enviar una o más apuestas |
| `0x03` | `END_BETS` | Cliente a servidor | Notificar que no se enviarán más apuestas |
| `0x80` | `ACK` | Servidor a cliente | Confirmar el procesamiento de un mensaje |
| `0x81` | `WINNER` | Servidor a cliente | Informar una apuesta ganadora |
| `0x82` | `WINNERS_END` | Servidor a cliente | Finalizar la secuencia de ganadores |
| `0xFF` | `ERROR` | Servidor a cliente | Informar un error de protocolo o procesamiento |

### Representación de una apuesta

Una apuesta no incluye el identificador de agencia porque este queda asociado a
la sesión mediante el mensaje `AGENCY`.

| Campo | Representación |
|---|---|
| Nombre | Longitud `uint16` seguida de bytes UTF-8 |
| Apellido | Longitud `uint16` seguida de bytes UTF-8 |
| Documento | `uint64` |
| Fecha de nacimiento | Longitud `uint16` seguida de bytes UTF-8 |
| Número apostado | `uint32` |

El decodificador rechaza strings que no sean UTF-8, campos incompletos y payloads
que contengan bytes sin consumir.

### Payloads

#### `AGENCY`

Contiene únicamente el identificador de agencia como `uint32`.

#### `BETS`

Comienza con la cantidad de apuestas como `uint32`, seguida por esa cantidad de
apuestas serializadas. La cantidad máxima de registros agrupados por el cliente se
configura mediante `BATCH_SIZE`; el último mensaje puede contener una cantidad
menor si el archivo no completa otro lote.

No se admite una cantidad igual a cero. El servidor decodifica y valida todo el
payload antes de intentar almacenarlo.

### Procesamiento por lotes

El cliente requiere que `BATCH_SIZE` esté definido y valida que sea un entero
positivo. Lee y convierte los registros de a uno, manteniendo en memoria únicamente
el lote actual. Cuando alcanza el tamaño configurado, lo serializa dentro de un
solo mensaje `BETS`, espera su `ACK` y reutiliza el espacio para el lote siguiente.
No se dividen apuestas entre mensajes ni se agrega padding para completar un
tamaño fijo.

Si una fila no posee la estructura CSV esperada, contiene campos numéricos
inválidos o no puede representarse con el formato del protocolo, el cliente la
omite y registra el índice y el motivo. Las demás apuestas continúan procesándose y
la fila inválida nunca se incorpora a un mensaje `BETS`.

El servidor deserializa y valida todas las apuestas antes de invocar
`Lottery.store_bets` una sola vez con la lista completa. El `ACK` se envía solamente
si esa llamada finaliza correctamente e informa la misma cantidad de registros que
el cliente envió. Un error de decodificación impide que el lote llegue a la capa de
dominio; un error de almacenamiento produce `ERROR` y no una confirmación exitosa.

Un payload malformado detectado por el servidor se rechaza completo y la sesión
finaliza. Como las apuestas tienen campos de longitud variable y no poseen una
longitud total individual, después de un error estructural no siempre es posible
ubicar con seguridad el inicio de la apuesta siguiente. Tampoco se retransmite
automáticamente el mismo lote: un error de protocolo volvería a producir el mismo
resultado y un reintento tras un fallo de almacenamiento podría duplicar registros.
La interfaz provista por `Lottery` no ofrece una operación de rollback, por lo que
ante un fallo de escritura el servidor no confirma el lote, aunque tampoco puede
deshacer registros que el método hubiera alcanzado a persistir antes de informar
el error.

## Concurrencia y sincronización

### Modelo de ejecución

El servidor emplea un proceso padre coordinador y un proceso hijo por conexión.
El padre no procesa mensajes de clientes: utiliza
`multiprocessing.connection.wait` para esperar simultáneamente sobre el socket de
escucha, las conexiones de control y los descriptores que informan la finalización
de los hijos. De esta forma puede aceptar conexiones, actualizar el quorum y
recolectar procesos sin polling. Cada hijo es dueño del socket aceptado durante
toda la sesión y lo cierra al finalizar.

Los workers se crean con `multiprocessing` usando explícitamente el método
`spawn`. Cada proceso comienza con un intérprete nuevo y recibe solamente el
socket TCP, el extremo de su conexión de control y la configuración de
almacenamiento que necesita. El padre cierra sus copias del socket aceptado y del
extremo destinado al hijo inmediatamente después de iniciarlo. Los procesos
terminados se recolectan mediante `join`, evitando procesos zombie.

Se eligió multiprocessing para permitir paralelismo real entre sesiones y evitar
que el GIL de CPython condicione la ejecución de sus tramos de procesamiento. No se
utilizan `Queue`, `Manager`, futures, asyncio ni memoria Python compartida. La
biblioteca `multiprocessing` crea los procesos y proporciona los objetos `Pipe` y
`Connection` empleados para el IPC, además del `Lock` que protege el almacenamiento
compartido.

### Protección de Lottery

Los accesos al almacenamiento de `Lottery` se coordinan mediante un único
`multiprocessing.Lock`, creado con el mismo contexto `spawn` que los workers y
compartido con todos ellos. Cada llamada a `store_bets` mantiene el lock hasta
finalizar. La iteración de `load_bets` también lo conserva durante todo el recorrido,
por lo que ningún proceso puede modificar ni interpretar simultáneamente un CSV
parcial. Ambas secciones críticas usan el context manager del propio `Lock`, que lo
libera al salir del bloque aunque ocurra una excepción. Este mutex simplifica la
sincronización a cambio de serializar también las
lecturas de distintas rondas. Las apuestas continúan procesándose de manera
incremental y no se carga el archivo completo en memoria.

### Quorum de agencias

El mensaje `END_BETS` actúa como notificación de que una agencia terminó de cargar
sus apuestas. El mínimo se obtiene de la variable obligatoria
`AGENCY_QUORUM_MIN`, que debe ser un entero positivo. El coordinador agrupa las
notificaciones en rondas sucesivas de exactamente esa cantidad de agencias
distintas; dos conexiones con el mismo identificador no ocupan dos lugares dentro
de una misma ronda.

Cada worker posee un `multiprocessing.Pipe` dúplex y conserva uno de sus extremos;
el otro pertenece al padre. Después de recibir `END_BETS`, el hijo envía el
`agency_id` como cuatro bytes mediante `send_bytes` y queda bloqueado en
`recv_bytes`. Como el conjunto de agencias finalizadas pertenece solamente al
padre, no necesita memoria compartida ni un lock adicional. El padre consume cada
notificación y la deja en la cola de la próxima ronda. Cada vez que se alcanza el
mínimo, selecciona exactamente esos workers y les envía un token de un byte. Las
agencias restantes comienzan a conformar otra ronda.

`Connection` conserva los límites de cada mensaje y el receptor valida que la 
notificación mida cuatro bytes y que el token sea el acordado. Tanto `recv_bytes` 
como `multiprocessing.connection.wait` son bloqueantes: no hay busy wait ni períodos
prefijados. Una nueva ronda completa puede liberarse aunque otras todavía estén
procesando ganadores, maximizando el paralelismo entre procesos; solamente queda
bloqueado el grupo incompleto que aún no alcanza el quorum. El padre asocia cada
PID con su ronda y observa los sentinels para registrar su finalización y recolectar
todos sus procesos de manera independiente. Los locks compartidos permiten que
varias rondas lean ganadores concurrentemente, mientras que el lock exclusivo
continúa impidiendo lecturas durante una escritura.

Si quedan menos agencias que el mínimo, sus workers permanecen bloqueados sin
consumir CPU hasta que lleguen las notificaciones faltantes. El quorum no se
relaja mediante un timeout; si nunca se completa, solamente una terminación del
sistema mediante `SIGTERM` interrumpe esa espera y libera sus recursos.

El sorteo se calcula por sesión y los resultados se filtran por `agency_id`. No se
realiza un broadcast global de ganadores.

#### `END_BETS`

No posee payload.

#### `ACK`

Contiene el tipo confirmado como `uint8` y la cantidad procesada como `uint32`.
Para `AGENCY` la cantidad es cero. Para `BETS` debe coincidir con la cantidad de
apuestas enviada. El servidor envía el `ACK` solamente después de completar la
operación correspondiente.

#### `WINNER`

Contiene una apuesta serializada. La agencia no se incluye porque el servidor
envía solamente ganadores asociados a la sesión actual.

#### `WINNERS_END`

Contiene como `uint32` la cantidad total de mensajes `WINNER` enviados. Esto
permite que el cliente valide que recibió la secuencia completa.

#### `ERROR`

Contiene el tipo de mensaje que falló como `uint8`, un código de error `uint16`,
la longitud del detalle como `uint16` y el detalle codificado en UTF-8. El tipo
`0x00` representa un error ocurrido antes de poder identificar el mensaje.

Los códigos definidos son:

| Código | Significado |
|---:|---|
| `1` | Encabezado o payload malformado |
| `2` | Mensaje inesperado para el estado actual |
| `3` | Datos de agencia o apuesta inválidos |
| `4` | Error interno durante el procesamiento |

Después de enviar o recibir un `ERROR`, la sesión se considera fallida y se cierra.

### Flujo y máquina de estados

El servidor mantiene los siguientes estados por conexión:

1. `WAITING_AGENCY`: solamente acepta `AGENCY` y responde `ACK`.
2. `RECEIVING_BETS`: acepta cero o más mensajes `BETS`, responde un `ACK` por cada
   uno y finalmente acepta `END_BETS`.
3. `WAITING_QUORUM`: registra la agencia terminada y espera a que se alcance
   `AGENCY_QUORUM_MIN`.
4. `SENDING_WINNERS`: recorre las apuestas almacenadas, filtra por la agencia de
   la sesión y aplica la condición de ganador. Envía cada resultado mediante
   `WINNER` y termina con `WINNERS_END`.
5. `FINISHED`: la comunicación de la sesión terminó y se cierra el socket.

El diálogo normal es:

```text
Cliente                                      Servidor
   | ----------- AGENCY ----------------------> |
   | <---------- ACK --------------------------- |
   | -------- BETS (hasta BATCH_SIZE) --------> |
   | <---------- ACK --------------------------- |
   |                      ...                    |
   | ----------- END_BETS --------------------> |
   |          (espera bloqueante del quorum)     |
   | <---------- WINNER ------------------------ |
   |                      ...                    |
   | <---------- WINNERS_END ------------------- |
```

Un mensaje fuera de orden produce `ERROR`. La sincronización normal depende del
intercambio de mensajes y no de esperas temporales prefijadas.

### Finalización graceful

Tanto el cliente como el servidor registran `SIGTERM` en sus respectivos
entrypoints y lo convierten en un pedido de terminación controlado. De esta manera
los módulos internos retornan errores o desenrollan la pila en lugar de invocar
funciones de salida forzada.

En Go, `os/signal` entrega `SIGTERM` por un canal y una goroutine lo traduce al
cierre de un canal de notificación propio. Los reintentos esperan ese canal con
`select`; cada intento de conexión posee un timeout de un segundo para mantener
acotada una terminación que ocurra durante `Dial`. Otra goroutine cierra la
conexión TCP para desbloquear cualquier lectura o escritura en curso. Ambas
goroutines disponen de canales de finalización que el proceso espera antes de
retornar. Un `sync.Once` evita que el cierre normal y la señal cierren la conexión
más de una vez. Los archivos de entrada y salida se liberan mediante `defer`; el
writer CSV se vacía también si la recepción de ganadores es interrumpida.

En Python el handler convierte `SIGTERM` en `ShutdownRequested`. El proceso padre
sale de `multiprocessing.connection.wait`, cierra el socket de escucha y solicita
la terminación de todos los workers mediante `Process.terminate`, que en POSIX les
entrega la misma señal. Cada hijo instala su propio handler, por lo que la señal
desenrolla la pila; los context managers y bloques `finally` liberan el socket TCP,
la `Connection` de IPC y el lock de `Lottery` si el worker lo había adquirido.

El coordinador propaga `SIGTERM` a todos los workers antes de comenzar a esperarlos,
por lo que sus limpiezas avanzan concurrentemente. Luego ejecuta `join` sobre cada
uno y cierra los objetos `Process` y los extremos de IPC retenidos por el padre.
Finalmente se eliminan los archivos de almacenamiento y, si el servidor tuvo que
crear su directorio interno, también lo elimina.

### Manejo de errores y recursos

La finalización de una lectura o escritura se determina al acumular exactamente la
cantidad esperada de bytes. Si una operación no avanza pero tampoco informa un
error, no se considera completa mientras todavía falten bytes y se vuelve a
intentar. Los errores informados por el socket antes de completar un encabezado 
o payload se propagan. Cuando la conexión todavía es utilizable, el servidor intenta 
enviar un mensaje `ERROR`; luego registra el problema y cierra la sesión.

Los errores se devuelven por el flujo normal de las funciones, sin forzar la salida
desde los módulos internos. Los archivos y sockets quedan bajo mecanismos de
cierre garantizado de cada lenguaje.
