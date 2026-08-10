# Por qué la GUI en Go necesita un compilador C (y PySide6 no)

## Pregunta original

`go run ./cmd/etl-gui` falló con `cgo: C compiler "gcc" not found` hasta
instalar un toolchain MinGW-w64. En Python, `pip install pyside6` da una
GUI Qt funcional sin instalar ningún compilador. ¿Por qué la misma clase
de programa (una ventana con un file picker) tiene requisitos tan
distintos en cada lenguaje?

## Hipótesis

No es que Go sea "peor" para GUIs — es que **Python y Go resuelven la
distribución de binarios en una etapa distinta del ciclo de vida del
paquete**:

- `pip install` descarga un artefacto ya compilado para tu plataforma.
- `go build`/`go run` compilan el artefacto en tu máquina, en el momento.

Fyne (la librería de GUI usada en [`cmd/etl-gui`](../cmd/etl-gui/main.go))
dibuja su interfaz sobre OpenGL vía `go-gl/glfw`, un binding CGO a la
librería GLFW escrita en C. Ese código C tiene que compilarse contra tu
sistema operativo — de ahí la dependencia de `gcc`.

## Qué instala cada gestor de paquetes

| | `pip install pyside6` | `go get fyne.io/fyne/v2` |
|---|---|---|
| Qué descarga | Un **wheel binario** (`.whl`) con las `.dll` de Qt ya compiladas para tu SO/arquitectura/versión de Python exactas | **Código fuente** Go + bindings C (GLFW) |
| Quién compiló el binario | El mantenedor del paquete, una vez, para cada plataforma soportada, subido a PyPI | Nadie todavía — tu máquina lo compila en cada `go build` |
| Rol de tu compilador local | Ninguno. `pip` copia archivos | Central. `gcc` compila el código C, el linker de Go combina el resultado con el código Go |
| Análogo en Go | No existe un "wheel" estándar en el toolchain de Go | — |

`pip` funciona como un sistema de distribución de binarios precompilados
por plataforma. El toolchain estándar de Go no tiene ese concepto: `go
build` es, casi siempre, la única fuente de verdad — compila el árbol de
dependencias completo desde cero cada vez que cambia algo relevante
(versión de Go, de la dependencia, o el propio código).

## Por qué Go está diseñado así

Esto no es un descuido del ecosistema Fyne — es consistente con una
decisión de diseño central de Go: **el artefacto final debe ser un único
binario estático, reproducible, sin runtime externo que instalar**.

- Un `.exe` de Go compilado no necesita que el usuario final tenga Go
  instalado, ni las librerías de Fyne, ni gcc. Se copia y corre.
- PySide6, en cambio, exige que la máquina final tenga Python + el
  wheel de PySide6 (o que vos empaquetes todo con algo como PyInstaller,
  que en el fondo hace un trabajo equivalente: empaquetar el intérprete
  y las DLLs de Qt en un solo directorio para distribuir).

Es la misma tensión que aparece en otras partes de Go (compilación
estática, sin gestor de paquetes de sistema, sin dependencia de un
runtime instalado aparte): **se paga el costo de compilar una vez, a
cambio de un artefacto final sin dependencias que cargar**. Python
invierte esa relación — instalar es instantáneo porque el costo de
compilar ya lo pagó el mantenedor del paquete, pero distribuir el
resultado final a otra máquina requiere volver a resolver ese mismo
problema (PyInstaller, Docker, etc.).

## Por qué específicamente Fyne, y no todo Go

CGO no es intrínseco a "hacer una GUI en Go" — es intrínseco a **cómo
Fyne dibuja**. Fyne renderiza su interfaz vía OpenGL, y el binding a
OpenGL en Go pasa por C. Frontends Go que evitan esta capa no requieren
compilador C:

- **Wails** — usa el WebView nativo que ya trae el sistema operativo
  (WebView2 en Windows) en vez de dibujar con OpenGL. El costo de CGO
  puede ser menor según el setup, pero introduce un frontend en
  HTML/CSS/JS separado del código Go.
- **Bubble Tea** (TUI) — dibuja en la terminal con secuencias ANSI, sin
  tocar OpenGL ni GPU. Cero CGO, cero compilador C.
- **`net/http` + HTML servido localmente** — el "frontend" es un
  navegador ya instalado, no una librería gráfica embebida.

`cmd/etl` (el CLI) no necesita ninguna de estas capas y por eso compila
con `CGO_ENABLED=0` sin problema: no dibuja nada, solo procesa texto.

## Conclusión

El compilador C no es un requisito de Go como lenguaje — es el precio de
elegir una librería de GUI (Fyne) que renderiza vía OpenGL usando
bindings C, combinado con la filosofía de Go de resolver la compilación
en la máquina de destino en vez de distribuir binarios precompilados por
plataforma como hace PyPI. `pip install pyside6` se siente más simple
porque el trabajo de compilación ya fue hecho por otra persona, en otro
momento — no porque Qt sea más simple de compilar que GLFW.

## Cómo se resolvió en este proyecto

1. Instalado un toolchain MinGW-w64 vía
   `winget install BrechtSanders.WinLibs.POSIX.UCRT`.
2. `go env -w CGO_ENABLED=1` para no tener que exportarlo en cada sesión.
3. El instalador de winget agrega la ruta del compilador al **PATH de
   usuario de Windows**, pero esa variable solo se propaga a procesos
   *nuevos* — cualquier terminal ya abierta (incluida la integrada de un
   editor) sigue viendo el PATH viejo hasta reiniciarse.
4. Git Bash en particular no hereda el `Path` de Windows dentro de una
   sesión ya abierta ni a través de subshells del mismo proceso padre;
   necesitó además un `~/.bashrc` con el `export PATH=...` explícito
   hacia el `mingw64/bin` de WinLibs, cargado desde `~/.bash_profile`
   para que se aplique automáticamente en cada ventana nueva.

## Código de referencia

- [`cmd/etl-gui/main.go`](../cmd/etl-gui/main.go) — GUI Fyne, requiere CGO.
- [`cmd/etl/main.go`](../cmd/etl/main.go) — CLI, compila con
  `CGO_ENABLED=0` sin cambios.
- [`internal/pipeline/pipeline.go`](../internal/pipeline/pipeline.go) —
  lógica compartida por ambos frontends; no depende de CGO.
