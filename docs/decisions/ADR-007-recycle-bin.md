# ADR-007: Papelera y borrado en volúmenes sin papelera

**Estado:** aceptado (2026-11-18). La restauración en `C:` queda pendiente de evidencia. Revisión: al cerrar M2.

## Contexto

El PRD (§5.3) pide que el borrado vaya por defecto a la Papelera mediante
`IFileOperation` con `FOFX_RECYCLEONDELETE`. En los volúmenes sin papelera hay
que avisar de que el borrado será permanente. Falta decidir cómo sabe Lanbay
que un volumen no tiene papelera y qué hace la API en ese caso.

## Decisión

1. Cada volumen expone la capacidad `recycle`. Vale `true` solo si es `DRIVE_FIXED` y NTFS, la combinación verificada. En cualquier otro caso vale `false`; eso incluye la microSD FAT32 y cualquier `DRIVE_REMOVABLE`.
2. La capacidad se decide **antes** de borrar. Nunca se deduce del resultado de `IFileOperation`.
3. **El servidor aplica la regla**, no solo la UI. Un `DELETE` sin `permanent=true` sobre un volumen con `recycle=false` se rechaza con un error tipado (`recycle_unavailable`). La UI muestra entonces la confirmación de borrado permanente y repite la petición con `permanent=true`.
4. En los volúmenes con `recycle=true`, el borrado normal usa `IFileOperation` con `FOFX_RECYCLEONDELETE` y flags que no muestran UI. La combinación exacta se valida en M2 desde la tarea de ADR-004. El borrado permanente explícito no usa la Papelera.

## Alternativas

| Opción | Ventajas | Desventajas | Cuándo conviene |
|---|---|---|---|
| Llamar a `IFileOperation` y confiar en que falle si no hay papelera | Sin lógica propia | **Falso:** en la microSD la llamada no abortó y el archivo desapareció. Habría borrado permanente sin aviso | Nunca |
| Deducir la capacidad de que exista `$Recycle.Bin` | Señal observable | La carpeta puede no existir aún en un volumen fijo sin uso. Su presencia no garantiza el reciclaje (`spikes/recycle-bin/README.md`) | Solo como dato de diagnóstico |
| **Regla por tipo de unidad y sistema de archivos, aplicada en el servidor** (elegida) | Falla segura: ante la duda, se avisa y se pide confirmar | Puede pedir confirmación de más en volúmenes que sí tendrían papelera | Por defecto |

## Evidencia

`docs/spikes/results/2026-11-18/recycle-bin/recycle-bin.json` (`IFileOperation`,
flag `0x00080000`) y el inventario de ADR-006:

- `C:\` (NTFS, `DRIVE_FIXED`): `$Recycle.Bin` presente, `aborted=False`, el archivo desapareció de su ruta.
- `D:\` (microSD FAT32, `DRIVE_REMOVABLE`): sin `$Recycle.Bin`, `aborted=False`, el archivo desapareció. Ahí no hay papelera, así que **el borrado fue permanente y la API no lo indicó**.

**No verificado:**

- Que el archivo de `C:` apareciera en la Papelera y se pudiera restaurar. El JSON solo prueba que salió de su ruta.
- El comportamiento desde la tarea oculta. El spike corrió en un PowerShell elevado visible.
- Si `D:` carece de papelera por ser removible o por ser FAT32. Ambos factores coinciden en la Ally. Por eso la regla exige `DRIVE_FIXED` y NTFS a la vez.

## Consecuencias

- La microSD de la Ally siempre pide confirmación de borrado permanente. Coincide con lo que el PRD preveía para "algunas microSD".
- El API y la UI muestran `recycle` en cada volumen, para avisar antes de seleccionar archivos.
- Un volumen fijo con exFAT o ReFS se trata como permanente hasta tener evidencia. Es una restricción conservadora y se puede relajar.

## Ajuste del ROADMAP

- **M2**: tests de la regla de capacidad y del rechazo `recycle_unavailable` en el servidor.
- **M2** / `docs/manual-test.md`: desde la tarea, borrar en `C:` y comprobar que el elemento aparece en la Papelera y se restaura. Borrar en la microSD y comprobar que aparece la confirmación de borrado permanente. Ninguna ventana debe aparecer en la pantalla de la Ally.

## Condición de reapertura

- En `C:` el elemento no llega a la Papelera o no se puede restaurar desde la tarea.
- `IFileOperation` muestra UI o se bloquea en el contexto de ADR-004.
- Aparece evidencia de otros sistemas de archivos o tipos de unidad que justifique ampliar la regla.
