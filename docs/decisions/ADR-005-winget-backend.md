# ADR-005: backend de winget

**Estado:** aceptado con condiciones (2026-11-18). Revisión: al cerrar M4.

## Contexto

La CLI de winget no ofrece JSON fiable para `search` y `list`. El PRD (§5.5.2)
fija este orden de preferencia: módulo `Microsoft.WinGet.Client`, API COM
`Microsoft.Management.Deployment` desde Go y, como último recurso, parsear la
CLI. El backend vive detrás de `software.Backend` (ADR-002).

## Decisión

M4 implementa `software.Backend` sobre la **CLI de winget** y reduce al mínimo
lo que depende de su texto:

- Instalar, actualizar y desinstalar usan solo `--id <id> --exact --source winget` con `--silent --accept-package-agreements --accept-source-agreements --disable-interactivity`. El resultado se decide por el **código de salida**, nunca por el texto.
- Solo se parsea texto en la búsqueda y en las actualizaciones disponibles. El parser trabaja con **fixtures reales capturados en la Ally**.
- Si el parser no reconoce la salida, el error se muestra de forma explícita. Nunca se devuelve una lista vacía como si no hubiera resultados.
- Lanbay no instala el módulo de PowerShell ni PowerShell 7 en la Ally.

## Alternativas

| Opción | Ventajas | Desventajas | Cuándo conviene |
|---|---|---|---|
| **CLI + códigos de salida + parser con fixtures** (elegida) | Es la única opción **verificada** en la Ally. Sin dependencias nuevas | El texto puede cambiar con la versión, el idioma o el ancho de consola | Mientras sea el único backend que funciona en el equipo |
| Módulo `Microsoft.WinGet.Client` | Objetos estructurados (`ConvertTo-Json`) | **No está instalado.** Instalarlo exige PSGallery y NuGet, red y una dependencia nueva. No se verificó en la tarea elevada | Si un spike lo instala y valida en la tarea de ADR-004 |
| API COM desde Go | Estructurada y sin texto | La activación WinRT **falló**. `CGO_ENABLED=0` obliga a escribir a mano el ABI WinRT: mucho código y mucho riesgo | Si un spike demuestra la activación por `CoCreateInstance` y el coste es asumible |

## Evidencia

`docs/spikes/results/2026-11-18/winget-backend/`:

- CLI (`winget-backend.json`): tres búsquedas de `7zip` con `exit_code 0`, en 3 896, 349 y 297 ms. La primera fue en frío. Las cabeceras salieron en inglés (`Name Id Version Match`) en un sistema `es-ES` con consola `es-MX`.
- Módulo (`powershell-5.json`): `Microsoft.WinGet.Client no está instalado` en PowerShell 5.1.
- PowerShell 7 (`powershell-7.json`): `pwsh.exe no disponible`.
- COM/WinRT (`winget-backend.json`): `RoInitialize` correcto. La activación de `PackageManager` devolvió `-2147221164` (`0x80040154`, `REGDB_E_CLASSNOTREG`) y un puntero nulo.
- Contexto real (`execution-context.json`, ADR-004): `winget install` desde la tarea terminó con `exit_code 0`.

**Límite de la evidencia:** el spike solo probó la activación WinRT. La creación
por `CoCreateInstance` con el CLSID del servidor COM de winget **no se probó**.
Este ADR no afirma que la API COM sea inviable, sino que no está disponible por
la vía medida.

## Consecuencias

- Se aparta del orden del PRD: no hay módulo y la API COM no está demostrada. Se cumple el principio del PRD de no depender del texto para las operaciones que modifican el sistema.
- El idioma de la salida no es un contrato. El parser no puede suponer cabeceras en inglés ni en español: se prueba con fixtures de ambos idiomas.
- Hay que fijar en `docs/manual-test.md` la versión de winget usada en los fixtures. Un cambio de versión de winget obliga a revisar el parser.

## Ajuste del ROADMAP

- **M4** suma estas tareas: capturar en la Ally los fixtures de `search`, `list` y `upgrade` con nombres largos, y probar el parser con salidas truncadas.
- Spike opcional, que no bloquea M4: activar el COM de winget con `CoCreateInstance` desde Go, sin cgo, en la tarea elevada. Si funciona, se abre un ADR que sustituye a este.

## Condición de reapertura

- El parser falla con los fixtures de una versión nueva de winget y no se puede arreglar sin heurísticas frágiles.
- Un spike demuestra que el módulo o la API COM funcionan en el contexto de ADR-004 con un coste asumible.
