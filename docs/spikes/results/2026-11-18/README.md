# Evidencia M0 — ejecución del 2026-11-18 en la Ally

Salida de los cuatro spikes de `spikes/` ejecutados con el kit M0 (`main` en
`4f11642`) en la ROG Xbox Ally X de Daniel Jacobo. Es la segunda ejecución:
la primera (2026-11-17) quedó invalidada por un fallo de entrecomillado del kit,
corregido en el PR #16. QA validó esta ejecución en JCB-456.

Esta es la única fuente de evidencia de los ADR-004 a ADR-007.

## Entorno

| Dato | Valor |
|---|---|
| Windows | Windows 10 Home (según WMI), versión 2009, build 26300, idioma `es-ES` |
| App Installer / winget | 1.29.379.0 / v1.29.380 |
| PowerShell | 5.1.26100.9818; PowerShell 7 no instalado |
| `C:` | NTFS, `DRIVE_FIXED` |
| microSD | `D:`, FAT32, `DRIVE_REMOVABLE`, 32 GB |

## Archivos

| Archivo | Spike | ADR |
|---|---|---|
| `environment.json` | entorno | todos |
| `execution-context/execution-context.json` | contexto de ejecución | ADR-004 |
| `winget-backend/winget-backend.json`, `powershell-5.json`, `powershell-7.json` | backend winget | ADR-005 |
| `with-microsd.json`, `without-microsd.json` | volúmenes | ADR-006 |
| `recycle-bin/recycle-bin.json` | Papelera | ADR-007 |

## Lo que no se ejecutó

- MSI silencioso e instalador interactivo: las rutas quedaron vacías (`exit_code: -1`, `not run`).
- Casos de red del spike de volúmenes (`subst` sobre UNC, `net use`, symlink a UNC): no hay `network-cases.json`.
- Confirmación visual de la Papelera y restauración: el JSON no la registra.

Cada ADR trata estos huecos como condición pendiente; ninguno los da por verificados.

## Integridad y redacción

Origen: `lanbay-m0-results-20261118144419.zip`, adjunto en JCB-456,
SHA-256 `1e322670fe30836edc545770f95eb5f50d5bf26cbba2fc5232bc081c9c5c290c`.

Los archivos son copias byte a byte del ZIP, salvo `execution-context.json`.
En ese archivo se sustituyeron por `<redactado>` la cuenta Microsoft, el SID
de esa cuenta y el nombre de usuario local, porque el repositorio es público.
La sustitución conserva el ancho de columna. Las rutas del ZIP usaban `\` y se
normalizaron a `/`. Ningún dato técnico se alteró.

SHA-256 de los originales, antes de redactar:

```text
a701172d4217fb743b0bcb832296f8ffd5fedf83d5b53fd6795953a4d21ea329  environment.json
257072bc32b591e287b778f6f09a019a9c28a3b6ea4e88c7b9b84f684d1d54bb  execution-context/execution-context.json
d92cfb8861385f38b9eb37327860ea2c67e855081564b4165f2210c4af105b43  recycle-bin/recycle-bin.json
642c9eacc6d22e957c00dd6215969d56e997af802e41d5c8a376b1084c7a1160  winget-backend/powershell-5.json
a0ccf492baf6166bd50d95c1e7574544276b73fb9179e2445a4c539d4e978ec4  winget-backend/powershell-7.json
397f7d9c7d9bcbfbbd97d3a33d82c92ec9c75a98860b6d81496a95a45f00e4da  winget-backend/winget-backend.json
6b835130208658e590499f8c3609f9f67b6501dc091c896e911a190c33cbf098  with-microsd.json
966ddaab82ea76dc627e67898a9f0bfd4c683d04dfefe6acd175517e926211e1  without-microsd.json
```
