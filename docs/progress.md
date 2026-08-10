# Mini ETL — progreso

Proyecto de aprendizaje de Go: ETL sobre `data/olist_customers_dataset.csv`
(dataset público Olist, ~99,441 filas de clientes).

Pipeline objetivo: **Extract → Transform → Load**, con logging y timing
como cross-cutting concerns (no acoplados a ninguna etapa).

---

## Estado actual

- [x] Struct `Customer` + validación
- [x] Extract (CSV → canal de `Customer`, streaming, goroutine única)
- [x] Logging de errores desacoplado (paquete propio, consumido vía canal)
- [x] Timer de fases (paquete propio, patrón `defer timer.Track(...)()`)
- [x] Transform (normalización + agregación por cliente único)
- [x] Load (JSON Lines a `data/output/customers_summary.jsonl`, streaming)
- [x] Benchmark: streaming vs. slice intermedio en Load (ver
      [benchmarks/streaming-vs-buffered-load.md](benchmarks/streaming-vs-buffered-load.md))
- [x] Tests unitarios (`customer.Validate`, `transform.Normalize`, `transform.Aggregate`)
- [x] Load alternativo a Postgres (`-target=postgres`), batching transaccional + upsert idempotente

---

## Decisiones de diseño

| Decisión | Elección | Por qué |
|---|---|---|
| `ZipCodePrefix` | `string`, no `int` | No se hace aritmética con el zip; un `int` perdería ceros a la izquierda |
| Visibilidad de campos | Exportados (mayúscula) | `Customer` vive en su propio paquete; otros paquetes (`extract`, `transform`) necesitan acceso |
| Validación | Método `Validate() error` en el struct | Acopla la regla de "qué es válido" al tipo, no a quien lo procesa |
| Extract | `<-chan Customer` + `<-chan error` (dos canales) | Separa datos de errores; no impone política de logging dentro de `extract` |
| Logging | Paquete `internal/logging`, consume `<-chan error` | Cross-cutting: reutilizable por `transform` y `load` sin cambios |
| Timing | Paquete `internal/timer`, patrón `defer timer.Track(stage)()` | Cross-cutting, igual que logging; mide fases sin instrumentar la lógica de negocio |
| Sincronización en `main` | `sync.WaitGroup` solo para el logger | El canal `customers` se espera implícitamente con `for range`; el logger corre en goroutine aparte y necesita esperarse explícitamente |
| Dedup en Transform | Agregar (contar pedidos por `unique_id`), no descartar filas | El CSV es 1 fila = 1 pedido; descartar perdería información. Se modela como `CustomerSummary` derivado, no como `Customer` filtrado |
| Ubicación de `CustomerSummary` | Paquete `transform`, no `customer` | Es un artefacto derivado del pipeline (decisión de agregación), no una entidad de dominio "cruda" como `Customer` |
| Transform: dos sub-etapas | `Normalize` (streaming, fila a fila) + `Aggregate` (bloqueante, requiere canal agotado) | Agregar por `unique_id` exige ver todas las filas antes de emitir; normalizar no. Se separan porque tienen restricciones distintas |
| Title case de `City` | `golang.org/x/text/cases` (no `strings.Title`, deprecado) | `strings.Title` no maneja bien Unicode; primera dependencia externa del proyecto, justificada por corrección de Unicode en nombres de ciudad |
| Load: formato y ruta | JSON en `data/output/customers_summary.json` | Preserva tipos, fácil de inspeccionar; subcarpeta separada del CSV crudo de entrada |
| Load: síncrono, sin canales | Función simple `JSON(path, summaries) error` | Para cuando Load corre, el resultado ya está completo en memoria (poblado por `Aggregate`); streaming no aporta nada sobre un slice ya acotado |
| Errores de Load | `log.Fatalf`, no logging-y-seguir | Distinto a errores de fila: un fallo al escribir la salida es fatal para el pipeline, no hay "siguiente fila" que procesar |
| Tags JSON en `CustomerSummary` | `snake_case` explícito (`customer_unique_id`, etc.) | Sin tags, `encoding/json` expondría los nombres Go (`UniqueID`) tal cual; se prefiere coherencia con las columnas originales del CSV |

---

## Estructura del proyecto

```
go_proy1/
├── go.mod
├── cmd/
│   └── etl/
│       └── main.go          # orquesta el pipeline
├── internal/
│   ├── customer/
│   │   ├── customer.go      # struct Customer + Validate()
│   │   └── customer_test.go
│   ├── extract/
│   │   └── csv.go           # lee el CSV, produce dos canales
│   ├── logging/
│   │   └── logging.go       # drena canales de error
│   ├── timer/
│   │   └── timer.go         # mide duración de fases
│   ├── transform/
│   │   ├── transform.go     # Normalize (streaming) + Aggregate (bloqueante)
│   │   └── transform_test.go
│   └── load/
│       ├── json.go          # escribe canal de CustomerSummary a JSON Lines
│       └── json_bench_test.go
└── data/
    ├── olist_customers_dataset.csv
    └── output/
        └── customers_summary.jsonl
```

---

## Próximos pasos

1. (Futuro, fuera de alcance inicial) Load alternativo a SQLite/Postgres.
2. (Futuro) Tests para `extract` (requiere CSV de prueba o `io.Reader` inyectable).

---

## Log de sesiones

### 2026-08-08
- Definido struct `Customer` y validación.
- Implementado `extract` (streaming vía goroutine + dos canales).
- Implementado `logging` como módulo desacoplado.
- Pipeline end-to-end probado: 99,441/99,441 filas válidas, sin errores.
- Añadido `internal/timer` y timing en `main.go`.

### 2026-08-09
- Implementado `transform`: `Normalize` (title-case vía `golang.org/x/text/cases`,
  valida `State`) + `Aggregate` (agrupa por `customer_unique_id`, cuenta pedidos).
- `CustomerSummary` definido en `transform`, no en `customer` (artefacto derivado).
- Pipeline extendido a 3 etapas concurrentes: `Extract → Normalize → Aggregate`.
- Resultado: 99,441 filas de pedidos → 96,096 clientes únicos.
- Primera dependencia externa añadida: `golang.org/x/text`.
- Implementado `load`: `JSON()` síncrono, escribe `[]CustomerSummary` con
  tags `snake_case` a `data/output/customers_summary.json`.
- Cambiado a `JSONLines()`: consume el canal de `Aggregate` directamente
  (sin acumular en slice intermedio), escribe un objeto JSON compacto por
  línea con `bufio.Writer`. Motivo: el JSON pretty-print con array producía
  576,578 líneas (~9-10MB con indentación); JSONL da 96,096 líneas (una por
  registro), formato estándar para pipelines de datos (S3/Athena/Glue) y
  permite streaming real de escritura sin esperar el resultado completo.
- Pipeline ETL completo end-to-end probado. Tiempos: transform ~102ms,
  load ~2ms, pipeline total ~227ms (incluye overhead de arranque de goroutines).
- Renombrado el timer de la fase final de `"load"` a `"extract-transform-load"`:
  como `JSONLines` ahora consume el canal de `Aggregate` directamente, medir
  ese bloque mide todo el streaming concurrente (Extract→Normalize→Aggregate→Load
  corriendo a la vez), no solo la escritura a disco.
- Evaluado mover `logging`/`timer` a `internal/utils`: descartado. La stdlib
  de Go evita paquetes `utils` genéricos (sin cohesión propia); se mantienen
  como `internal/logging` e `internal/timer`, nombrados por responsabilidad.
- Nota de robustez pendiente: `os.Create` en `load.JSONLines` no crea el
  directorio padre si no existe (a diferencia de herramientas de alto nivel).
  Pendiente decidir si añadir `os.MkdirAll` antes de escribir.
- Confirmado empíricamente el porqué de la mejora ~259.8ms → ~149.98ms:
  benchmark en `internal/load/json_bench_test.go` comparando streaming puro
  vs. slice intermedio (funciones internas `writeJSONLines`/`writeJSONArray`,
  `io.Discard` para aislar CPU/memoria del I/O real). Resultado: streaming es
  ~3.05x más rápido y usa ~15.6x menos memoria (8.48MB vs 131.93MB por op),
  porque evita materializar los 96,096 registros completos en un slice antes
  de serializar. Detalle completo en
  [benchmarks/streaming-vs-buffered-load.md](benchmarks/streaming-vs-buffered-load.md).
- Añadidos tests table-driven: `customer_test.go` (5 casos de `Validate`),
  `transform_test.go` (`Normalize` con fila válida/inválida, `Aggregate`
  con `unique_id` repetido). Todos pasan; `go run ./cmd/etl` verificado
  de nuevo sin regresiones.
- Creado `README.md` en la raíz del proyecto, describiendo el pipeline
  para alguien que llega sin contexto.
- Añadido Load a Postgres: `internal/config` (carga `.env.local` vía
  `godotenv` + `os.Getenv`, valida credenciales requeridas),
  `internal/load/postgres.go` (`database/sql` + driver `pgx/v5/stdlib`,
  batching de 1000 filas por transacción, `ON CONFLICT DO UPDATE` para
  idempotencia), `docs/sql/schema.sql` (DDL de `customer_summary`).
  `main.go` gana flag `-target=jsonl|postgres` (default `jsonl`, sin
  romper el comportamiento previo).
- Probado end-to-end contra Postgres local real: 96,096 filas cargadas
  en ~7.3s (vs ~130ms para JSON Lines — esperable: red + SQL parsing +
  transacciones vs. escritura secuencial a disco). Corrida repetida sin
  duplicar filas, confirmando que el upsert funciona.
- Debug real: `.env.local` tenía `PGDATABASE` igual al nombre de usuario
  (`go_inventory_user`) en vez del nombre real de la base
  (`go_inventory`) — error `3D000 no existe la base de datos`,
  diagnosticado listando bases con `psql -c "\l"`.
