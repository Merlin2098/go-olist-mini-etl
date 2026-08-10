# Arquitectura

Este documento describe cómo está compuesto el proyecto y cómo fluyen
los datos, para alguien que llega sin contexto previo. Para el
historial de decisiones de diseño (por qué se eligió cada cosa), ver
[../progress.md](../progress.md).

## Vista general

```
                    ┌─────────────┐   ┌──────────────┐
                    │  cmd/etl     │   │ cmd/etl-gui  │
                    │  (CLI)       │   │ (Fyne GUI)   │
                    └──────┬───────┘   └──────┬───────┘
                           │                  │
                           └────────┬─────────┘
                                    ▼
                        ┌────────────────────┐
                        │  internal/pipeline  │
                        │  (orquesta Run)     │
                        └──────────┬──────────┘
                                    ▼
        ┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐
        │ extract    │──▶│ transform  │──▶│ transform  │──▶│  load      │
        │ (CSV)      │   │ Normalize  │   │ Aggregate  │   │ jsonl /    │
        │            │   │            │   │            │   │ postgres   │
        └───────────┘   └───────────┘   └───────────┘   └───────────┘
```

`cmd/etl` y `cmd/etl-gui` son las dos únicas capas que conocen al
usuario final (flags de línea de comandos o widgets). Ninguna
reimplementa lógica de negocio: ambas arman un `pipeline.Options` y
llaman a `pipeline.Run`. Esto es intencional — agregar un tercer
frontend (por ejemplo, un endpoint HTTP) no debería requerir tocar
`extract`, `transform` o `load`.

## Flujo de datos

El pipeline es una cadena de etapas conectadas por canales de Go
(`chan`), cada una en su propia goroutine:

1. **Extract** (`internal/extract`) — lee el CSV fila por fila, produce
   un `Customer` por fila. Emite dos canales: `<-chan Customer` (datos
   válidos) y `<-chan error` (filas malformadas). El CSV es 1 fila = 1
   *pedido*, no 1 fila = 1 persona.
2. **Transform / Normalize** (`internal/transform`) — recibe
   `Customer`, valida `State` (debe tener 2 caracteres) y normaliza
   `City` a title case. Streaming puro: una fila entra, una fila (o un
   error) sale, sin esperar al resto.
3. **Transform / Aggregate** (`internal/transform`) — agrupa por
   `customer_unique_id` y cuenta pedidos. A diferencia de `Normalize`,
   esta etapa **no puede emitir nada hasta agotar el canal de
   entrada**: el conteo final de una persona no se conoce hasta ver su
   última fila.
4. **Load** (`internal/load`) — consume `<-chan CustomerSummary` y lo
   persiste en uno de dos destinos, seleccionado por
   `pipeline.Options.Target`:
   - `jsonl` → `load.JSONLines`, streaming directo a un archivo
     `.jsonl` (staging/replay portable).
   - `postgres` → `load.Postgres`, upsert en batches transaccionales a
     `customer_summary` (capa analytics-ready).

Errores de fila (CSV malformado, `state` inválido) no detienen el
pipeline — se acumulan en `Result.Errors` y cada frontend decide cómo
mostrarlos. Errores de infraestructura (no se pudo escribir el archivo,
no se pudo conectar a Postgres) sí son fatales y abortan el `Run`.

## Por qué dos destinos de Load

`jsonl` y `postgres` no son alternativas equivalentes — cumplen roles
distintos en un flujo de datos típico:

- **`jsonl`** es la capa de *staging/replay*: un snapshot inmutable y
  portable (formato que S3/Athena/Glue esperan consumir), sin
  dependencias externas.
- **`postgres`** es la capa *analytics-ready*: consultable con SQL,
  indexada por `customer_unique_id`, pensada para BI tools o
  agregaciones ad-hoc.

Ver la sección "Load: dos destinos, dos roles distintos" en el
[README](../../README.md) para más detalle.

## Cross-cutting concerns

`internal/logging` y `internal/timer` no pertenecen a ninguna etapa
específica del pipeline:

- `logging` consume cualquier `<-chan error` — reutilizable por
  cualquier etapa que exponga uno, sin cambios en esa etapa.
- `timer` mide duración con el patrón `defer timer.Track(nombre)()` —
  se puede envolver cualquier bloque sin instrumentar la lógica de
  negocio.

## Estructura de paquetes

```
cmd/
  etl/             CLI: flags -target=jsonl|postgres -source=<csv>
  etl-gui/         GUI de escritorio (Fyne)
internal/
  customer/        struct Customer + validación de dominio
  extract/         CSV → <-chan Customer, <-chan error
  transform/       Normalize (streaming) + Aggregate (agrupación)
  load/            JSON Lines y Postgres
  pipeline/        orquesta extract → transform → load
  config/          credenciales de Postgres (.env.local + env vars)
  logging/         cross-cutting: consume canales de error
  timer/           cross-cutting: mide duración de fases
```

`internal/pipeline` existe para que la orquestación se escriba una sola
vez: tanto `cmd/etl` como `cmd/etl-gui` son capas de entrada delgadas
que recolectan `pipeline.Options` y llaman a `pipeline.Run`.
