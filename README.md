# go_proy1 — mini ETL en Go

Pipeline ETL (Extract → Transform → Load) sobre el
[**Brazilian E-Commerce Public Dataset by Olist**](https://www.kaggle.com/datasets/olistbr/brazilian-ecommerce)
(Kaggle), un dataset público de pedidos de e-commerce brasileño.
Proyecto de aprendizaje de Go, construido incrementalmente como
ejercicio de idioms de concurrencia, manejo de errores y organización
de paquetes.

## Qué hace

Lee `data/olist_customers_dataset.csv` (~99,441 filas, una por *pedido*),
normaliza y agrega los datos por cliente único, y carga el resultado a
uno de dos destinos: un archivo JSON Lines o una tabla Postgres.

```
customer_id, customer_unique_id, customer_zip_code_prefix, customer_city, customer_state
```

↓

```json
{"customer_unique_id":"5ce16b5c...","customer_city":"Pinhais","customer_state":"PR","order_count":1}
```

El CSV es 1 fila = 1 pedido, no 1 fila = 1 persona: un mismo
`customer_unique_id` puede repetirse en varias filas. El pipeline agrupa
por persona y cuenta cuántos pedidos hizo (`order_count`), en vez de
simplemente deduplicar y descartar información.

## Arquitectura

Para el diagrama de componentes, el flujo de datos entre etapas y la
estructura de paquetes en detalle, ver
[docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md).

## Pipeline

```
Extract ──▶ Normalize ──▶ Aggregate ──▶ Load
 (CSV)      (title-case,   (agrupa por    (JSON Lines o Postgres,
             valida state)  unique_id)     según -target)
```

Cada etapa corre en su propia goroutine, conectada a la siguiente por
canales (`chan`). El dato fluye de una etapa a otra sin necesidad de
esperar a que una etapa anterior termine por completo — streaming real,
no lotes.

Errores de fila (CSV malformado, `state` inválido, etc.) no detienen el
pipeline: se envían por un canal de error separado y se loguean, mientras
las filas válidas siguen su curso. Errores de infraestructura (no se
pudo escribir el archivo de salida, no se pudo conectar a Postgres) sí
son fatales.

### Load: dos destinos, dos roles distintos

El flag `-target` decide el destino de Load, sin afectar el resto del
pipeline (`extract`/`transform` no saben ni les importa a dónde va el
resultado). Los dos destinos no son intercambiables al azar: cada uno
cumple un rol distinto en un flujo de datos típico.

- **`jsonl`** (default) — escribe streaming directo a
  `data/output/customers_summary.jsonl`, un objeto JSON compacto por
  línea. Es la capa de **staging/replay**: un snapshot inmutable del
  último run, en el formato que herramientas tipo S3/Athena/Glue
  esperan consumir. Sin dependencias externas, sin estado previo
  requerido — útil para verificar el pipeline en dev o CI sin depender
  de que Postgres esté disponible.
- **`postgres`** — upsert en batches de 1000 filas por transacción a la
  tabla `customer_summary` (ver [docs/sql/schema.sql](docs/sql/schema.sql)).
  Es la capa **analytics-ready**: consultable con SQL, indexada por
  `customer_unique_id`, pensada para conectarse a un BI tool o correr
  agregaciones ad-hoc. Usa `ON CONFLICT (customer_unique_id) DO
  UPDATE`, así que **re-correr el pipeline es seguro**: actualiza las
  filas existentes en vez de duplicarlas. Verificado empíricamente —
  tres corridas seguidas contra la misma base mantienen el conteo de
  filas constante.

En otras palabras: el JSON no compite con Postgres como destino
analítico final — es el registro portable/reproducible que antecede a
esa capa. La GUI simplemente expone esta misma elección (`jsonl` vs
`postgres`) sin agregar lógica propia.

## Estructura

```
cmd/
  etl/main.go             CLI: orquesta el pipeline, flag -target=jsonl|postgres -source=<csv>
  etl-gui/main.go         GUI de escritorio (Fyne): file picker + selector jsonl/postgres
internal/
  customer/               struct Customer + validación de dominio
  extract/                lee el CSV, produce <-chan Customer y <-chan error
  transform/               Normalize (streaming) + Aggregate (agrupación)
  load/                   JSON Lines (json.go) y Postgres (postgres.go)
  pipeline/               orquesta extract→transform→load; compartido por CLI y GUI
  config/                 carga credenciales de Postgres (.env.local + env vars)
  logging/                consume canales de error (cross-cutting, reutilizable)
  timer/                  mide duración de fases (cross-cutting, reutilizable)
data/
  olist_customers_dataset.csv    dataset de entrada
  output/customers_summary.jsonl resultado del pipeline (target=jsonl)
docs/
  progress.md                          bitácora de decisiones de diseño
  architecture/ARCHITECTURE.md         componentes, flujo de datos, estructura
  sql/schema.sql                       DDL de la tabla customer_summary
  sql/postgres-queries.md              queries de verificación y analítica
  benchmarks/streaming-vs-buffered-load.md    benchmark: streaming vs. slice intermedio
  benchmarks/go-vs-python-gui-distribution.md benchmark: distribución Go/CGO vs. Python
.env.local.example         plantilla de credenciales Postgres (.env.local no se versiona)
```

`internal/pipeline` existe para que la orquestación (extract → transform
→ aggregate → load) se escriba una sola vez. Tanto `cmd/etl` (flags de
CLI) como `cmd/etl-gui` (widgets de Fyne) son finas capas de entrada que
recolectan `pipeline.Options` y llaman a `pipeline.Run` — ninguna
reimplementa el wiring entre etapas.

`logging` y `timer` no pertenecen a ninguna etapa específica del ETL:
cualquier etapa que exponga un `<-chan error` puede conectarse al logger
sin cambios, y cualquier bloque de código puede medirse con
`defer timer.Track("nombre")()`. `logging` ya no lo usa `cmd/etl`
directamente (ahora `pipeline.Run` colecciona los errores en
`Result.Errors` para que cada frontend decida cómo mostrarlos), pero
queda disponible como pieza reutilizable.

## Requisitos

- Go 1.26+
- Postgres (solo si usas `-target=postgres` / destino PostgreSQL en la GUI)
- Para compilar `cmd/etl-gui`: CGO habilitado (`CGO_ENABLED=1`) y un
  compilador C en el `PATH` (en Windows, p. ej.
  [WinLibs MinGW-w64](https://winlibs.com/), instalable con
  `winget install BrechtSanders.WinLibs.POSIX.UCRT`). Fyne usa CGO para
  hablar con OpenGL/el sistema de ventanas; el CLI (`cmd/etl`) no lo
  necesita.

## Uso — CLI

```bash
go run ./cmd/etl                                     # target=jsonl (default)
go run ./cmd/etl -target=postgres                    # requiere .env.local + tabla creada
go run ./cmd/etl -source=ruta/a/otro.csv -target=jsonl # CSV alternativo
```

Imprime tiempos de ejecución y conteo de registros procesados.

Para usar `-target=postgres`:

1. Copia `.env.local.example` a `.env.local` y completa las credenciales.
2. Crea la base de datos y aplica el schema:
   ```bash
   psql -h localhost -U <usuario> -d <base> -f docs/sql/schema.sql
   ```
3. Corre `go run ./cmd/etl -target=postgres`.

Ver [docs/sql/postgres-queries.md](docs/sql/postgres-queries.md) para
queries de verificación una vez cargado.

## Uso — GUI de escritorio

```bash
go run ./cmd/etl-gui
```

La ventana permite:

1. **Browse…** — abre el selector de archivos nativo del sistema
   operativo para elegir el CSV de entrada (filtrado a `.csv`).
2. **Load target** — radio button para elegir `JSON Lines` o
   `PostgreSQL` como destino.
3. **Run pipeline** — ejecuta `internal/pipeline.Run` en una goroutine
   separada (la ventana no se congela) y muestra en el panel inferior
   el resultado: conteo de clientes cargados, errores de fila y
   errores fatales de carga.

Para `PostgreSQL` desde la GUI aplican los mismos requisitos que desde
el CLI: `.env.local` con credenciales y el schema ya aplicado (ver
sección anterior). La GUI no pide credenciales por pantalla — las lee
del entorno igual que el CLI.

## Tests

```bash
go test ./...                                    # todos los tests
go test ./internal/load/ -bench=. -benchmem -run=^$   # solo benchmarks de load
```

## Documentación adicional

- [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md) —
  diagrama de componentes, flujo de datos entre etapas y estructura de
  paquetes, pensado para alguien que llega sin contexto.
- [docs/progress.md](docs/progress.md) — bitácora de decisiones de diseño
  y su justificación (por qué `string` para zip code, por qué dos canales
  en `extract`, por qué `Aggregate` usa punteros en el map, etc.).
- [docs/sql/schema.sql](docs/sql/schema.sql) — DDL de `customer_summary`
  (`customer_unique_id` como `PRIMARY KEY`, lo que habilita el upsert).
- [docs/sql/postgres-queries.md](docs/sql/postgres-queries.md) —
  queries de verificación, analítica y mantenimiento sobre los datos
  cargados.
- [docs/benchmarks/streaming-vs-buffered-load.md](docs/benchmarks/streaming-vs-buffered-load.md) —
  benchmark que mide el costo de acumular resultados en un slice
  intermedio antes de escribirlos, contra escribir directamente desde
  el canal de la etapa anterior.
- [docs/benchmarks/go-vs-python-gui-distribution.md](docs/benchmarks/go-vs-python-gui-distribution.md) —
  por qué `cmd/etl-gui` (Fyne) necesita un compilador C y `pip install
  pyside6` no: la diferencia entre distribuir binarios precompilados
  por plataforma (PyPI) y compilar en la máquina de destino (Go/CGO).
