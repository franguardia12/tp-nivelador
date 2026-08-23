# Informe del TP Nivelador

## Arquitectura general

El sistema está compuesto por clientes escritos en Go que representan agencias de
lotería y un servidor escrito en Python que representa la central de Lotería
Nacional. Los procesos se comunican mediante sockets TCP y un protocolo binario
propio. No se utilizan bibliotecas de serialización ni formatos como JSON.

Cada cliente procesa su archivo de entrada de manera incremental. El servidor
persiste las apuestas mediante el modelo de dominio provisto y devuelve únicamente
los ganadores pertenecientes a la agencia que abrió la conexión.

## Protocolo de comunicación

### Convenciones

- Los enteros son sin signo y se codifican en orden de red (big-endian).
- Las longitudes de strings representan bytes UTF-8, no caracteres.
- Todos los mensajes poseen un encabezado fijo y un payload de longitud variable.
- El tamaño máximo de un payload es 16 MiB. Un encabezado que declare una longitud
  mayor se considera inválido y se rechaza antes de reservar memoria para el
  payload.
- `send_all` y `recv_all` se emplean para transferir exactamente la cantidad de
  bytes requerida, contemplando short writes y short reads.

### Framing

El encabezado tiene 5 bytes:

| Campo | Tamaño | Descripción |
|---|---:|---|
| Tipo | 1 byte | Identificador del mensaje |
| Longitud | 4 bytes | Cantidad de bytes del payload |

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
apuestas serializadas. En el ejercicio 5 cada mensaje contiene una sola apuesta.
Esta representación permite que el ejercicio de procesamiento por lotes aumente
la cantidad sin modificar el framing ni la serialización individual.

No se admite una cantidad igual a cero. El servidor decodifica y valida todo el
payload antes de intentar almacenarlo.

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
3. `SENDING_WINNERS`: recorre las apuestas almacenadas, filtra por la agencia de
   la sesión y aplica la condición de ganador. Envía cada resultado mediante
   `WINNER` y termina con `WINNERS_END`.
4. `FINISHED`: la comunicación de la sesión terminó y se cierra el socket.

El diálogo normal es:

```text
Cliente                                      Servidor
   | ----------- AGENCY ----------------------> |
   | <---------- ACK --------------------------- |
   | ----------- BETS (una apuesta) ----------> |
   | <---------- ACK --------------------------- |
   |                      ...                    |
   | ----------- END_BETS --------------------> |
   | <---------- WINNER ------------------------ |
   |                      ...                    |
   | <---------- WINNERS_END ------------------- |
```

Un mensaje fuera de orden produce `ERROR`. La sincronización normal depende del
intercambio de mensajes y no de esperas temporales prefijadas.

### Manejo de errores y recursos

Un EOF antes de completar un encabezado o payload se considera una comunicación
incompleta. Los errores de socket se propagan y no se reintentan de manera
indefinida. Cuando la conexión todavía es utilizable, el servidor intenta enviar
un mensaje `ERROR`; luego registra el problema y cierra la sesión.

Los errores se devuelven por el flujo normal de las funciones, sin forzar la salida
desde los módulos internos. Los archivos y sockets quedan bajo mecanismos de
cierre garantizado de cada lenguaje.
