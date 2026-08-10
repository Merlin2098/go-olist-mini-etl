# Queries de prueba — `customer_summary`

Queries para verificar y explorar los datos cargados por
`go run ./cmd/etl -target=postgres` en la tabla `customer_summary`
(schema: [schema.sql](schema.sql)).

Conexión:

```bash
psql -h localhost -p 5432 -U go_inventory_user -d go_inventory
```

---

## Verificación básica

```sql
-- ¿cuántas filas cargó el pipeline?
SELECT COUNT(*) FROM customer_summary;

-- muestra rápida
SELECT * FROM customer_summary LIMIT 10;

-- ¿customer_unique_id realmente es único? (debería ser 0 filas)
SELECT customer_unique_id, COUNT(*)
FROM customer_summary
GROUP BY customer_unique_id
HAVING COUNT(*) > 1;

-- ¿algún campo obligatorio quedó vacío? (debería ser 0 filas)
SELECT * FROM customer_summary
WHERE customer_city = '' OR customer_state = '' OR customer_unique_id = '';

-- ¿customer_state siempre tiene 2 caracteres?
SELECT DISTINCT customer_state, LENGTH(customer_state)
FROM customer_summary
WHERE LENGTH(customer_state) != 2;
```

---

## Analítica

```sql
-- top 10 clientes con más pedidos
SELECT customer_unique_id, customer_city, customer_state, order_count
FROM customer_summary
ORDER BY order_count DESC
LIMIT 10;

-- distribución de pedidos por cliente (¿cuántos clientes tienen 1, 2, 3... pedidos?)
SELECT order_count, COUNT(*) AS num_clientes
FROM customer_summary
GROUP BY order_count
ORDER BY order_count;

-- clientes únicos y pedidos totales por estado
SELECT customer_state,
       COUNT(*) AS clientes_unicos,
       SUM(order_count) AS pedidos_totales,
       ROUND(AVG(order_count), 2) AS promedio_pedidos_por_cliente
FROM customer_summary
GROUP BY customer_state
ORDER BY pedidos_totales DESC;

-- top 10 ciudades por cantidad de clientes únicos
SELECT customer_city, customer_state, COUNT(*) AS clientes_unicos
FROM customer_summary
GROUP BY customer_city, customer_state
ORDER BY clientes_unicos DESC
LIMIT 10;

-- % de clientes recurrentes (más de 1 pedido) vs. de un solo pedido
SELECT
    COUNT(*) FILTER (WHERE order_count = 1) AS un_pedido,
    COUNT(*) FILTER (WHERE order_count > 1) AS recurrentes,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE order_count > 1) / COUNT(*),
        2
    ) AS pct_recurrentes
FROM customer_summary;
```

---

## Mantenimiento / reset

```sql
-- vaciar la tabla para volver a correr el pipeline desde cero
TRUNCATE TABLE customer_summary;

-- eliminar la tabla por completo (el pipeline no la recrea; hay que
-- volver a aplicar docs/sql/schema.sql)
DROP TABLE customer_summary;

-- tamaño en disco de la tabla (útil para comparar contra el .jsonl)
SELECT pg_size_pretty(pg_total_relation_size('customer_summary'));
```

---

## Verificar idempotencia del upsert

Correr el pipeline dos veces seguidas no debería cambiar el conteo total
ni duplicar filas — confirma con:

```sql
SELECT COUNT(*) FROM customer_summary;
-- correr: go run ./cmd/etl -target=postgres
-- volver a correr el SELECT: el conteo debe ser idéntico
```
