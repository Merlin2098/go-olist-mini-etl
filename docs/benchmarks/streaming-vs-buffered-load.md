# Benchmark: Load streaming vs. Load con slice intermedio

## Pregunta original

Al cambiar `load.JSON` (recibe `[]CustomerSummary`, ya acumulado) por
`load.JSONLines` (recibe `<-chan CustomerSummary` directo de `Aggregate`),
el tiempo del pipeline completo bajó de ~259.8ms a ~149.98ms. ¿Por qué?

## Hipótesis

No es el formato de salida (JSON array pretty-print vs. JSON Lines
compacto) lo que domina el tiempo — es que la versión anterior hacía
**dos pasadas** sobre los 96,096 registros:

1. Drenar el canal de `Aggregate` a un slice (`append` en un loop,
   con relocaciones de memoria conforme el slice crece).
2. Iterar el slice completo para serializarlo como un único JSON array.

La versión streaming hace **una sola pasada**: cada registro sale del
canal y se serializa inmediatamente, sin estructura intermedia.

## Metodología

Benchmark aislado en `internal/load/json_bench_test.go`, comparando dos
funciones internas (`writeJSONArray` vs `writeJSONLines`) sobre datos
sintéticos (96,096 `CustomerSummary`, el mismo volumen que produce el
pipeline real), escribiendo a `io.Discard` en vez de a un archivo real.

**Por qué `io.Discard`:** para que el benchmark mida el costo de la
*estrategia de acumulación* (CPU, allocations), no la velocidad del
disco. Si escribiéramos a un archivo real, el I/O dominaría el tiempo
medido y ocultaría la diferencia entre las dos estrategias.

Comando:

```
go test ./internal/load/ -bench=. -benchmem -benchtime=20x -run=^$
```

## Resultados

| Métrica | `writeJSONArray` (slice) | `writeJSONLines` (streaming) | Diferencia |
|---|---|---|---|
| Tiempo/op | 77.66 ms | 25.43 ms | ~3.05x más rápido |
| Memoria/op | 131.93 MB | 8.48 MB | ~15.6x menos memoria |
| Allocations/op | 192,071 | 288,081 | Streaming aloja más veces, pero objetos mucho más chicos |

*(CPU: AMD Ryzen 5 8645HS, Windows, Go 1.26.4, `-benchtime=20x`)*

## Interpretación

- **La memoria es la señal más clara.** 131.93 MB vs 8.48 MB confirma
  la hipótesis: la versión con slice mantiene los 96,096 registros
  completos en memoria a la vez (el slice de `CustomerSummary`, más
  el buffer interno de `json.Marshal` serializando el array entero de
  una vez), mientras que la versión streaming nunca materializa más
  de un registro simultáneamente.
- **El conteo de allocations es engañoso si se mira solo.** La versión
  streaming aloja *más veces* (288,081 vs 192,071) — cada `enc.Encode`
  individual reserva su propio buffer temporal — pero son asignaciones
  pequeñas y de vida corta, mucho más baratas que las reasignaciones
  grandes que produce `append` haciendo crecer un slice de 96k elementos
  (Go duplica la capacidad del slice subyacente cada vez que se queda
  sin espacio, copiando todo el contenido anterior).
- **El tiempo total confirma que menos memoria movida gana**, incluso
  con más operaciones de allocación: mover menos bytes totales (~8MB
  vs ~132MB por operación) importa más que el número de allocations.

## Conclusión

Esto no es "JSON Lines es más rápido que JSON array" como propiedad del
formato — es la diferencia entre un pipeline con un **punto de
sincronización intermedio** (todo debe materializarse en un slice antes
de poder pasar a Load) y un pipeline **streaming end-to-end** (el dato
fluye de una etapa a la siguiente sin pararse a acumularse por completo).

Generaliza a cualquier etapa de un pipeline en Go: si una etapa puede
consumir directamente el canal de la etapa anterior en vez de esperar
a que esa etapa termine y entregue un slice, el pipeline se beneficia
tanto en throughput como en footprint de memoria — especialmente
notorio a medida que crece el volumen de datos.

## Código de referencia

- [`internal/load/json.go`](../internal/load/json.go) — `writeJSONLines`
  (estrategia actual, usada por `JSONLines`) y `writeJSONArray`
  (estrategia anterior, conservada solo como baseline de benchmark).
- [`internal/load/json_bench_test.go`](../internal/load/json_bench_test.go)
  — benchmarks.
