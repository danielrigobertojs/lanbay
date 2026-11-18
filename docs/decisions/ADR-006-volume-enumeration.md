# ADR-006: enumeración de volúmenes y exclusión de red

**Estado:** aceptado (2026-11-18). El caso de red queda pendiente de evidencia. Revisión: al cerrar M1 y M2.

## Contexto

Lanbay solo administra volúmenes locales (ADR-001). El PRD (§5.3) incluye
`DRIVE_FIXED` y `DRIVE_REMOVABLE` y excluye `DRIVE_REMOTE`, `DRIVE_NO_ROOT_DIR`
y `DRIVE_UNKNOWN`. Además pide verificar cómo se reporta un `subst` sobre una
ruta UNC, porque una letra creada así podría parecer local.

## Decisión

`volumes.Provider` en Windows usa una **lista de permitidos que falla cerrada**.
Una letra solo se acepta si cumple todo lo siguiente:

1. `GetDriveTypeW` devuelve `DRIVE_FIXED` o `DRIVE_REMOVABLE`. `DRIVE_CDROM` queda fuera del MVP.
2. `QueryDosDeviceW("X:")` devuelve **un único destino** que empieza por `\Device\HarddiskVolume`.
3. `GetVolumeInformationW` responde. Si no responde, la letra no se muestra.

Cualquier otro destino se excluye **sin intentar clasificarlo**: `\??\…` de
`subst`, `\Device\LanmanRedirector`, `\Device\Mup`, `\??\UNC\…` o un destino
desconocido. Así, `subst` sobre una ruta local o sobre UNC queda fuera aunque
`GetDriveTypeW` diga `DRIVE_FIXED`. Los volúmenes se enumeran de nuevo en cada
consulta, sin caché.

Los junctions y symlinks hacia UNC **dentro** de un volumen aceptado no son
cosa de la enumeración: los bloquea `pathguard` en M2 (PRD §5.4).

## Alternativas

| Opción | Ventajas | Desventajas | Cuándo conviene |
|---|---|---|---|
| Solo `GetDriveTypeW` (PRD) | Simple | `subst` hereda el tipo del destino. Que `subst` sobre UNC se reporte como `DRIVE_REMOTE` **no está verificado** | Nunca como única señal |
| `GetDriveTypeW` + lista de bloqueados de destinos de red | Muestra letras `subst` locales | Falla abierta: un prefijo de red no previsto se cuela | Si hiciera falta mostrar `subst` locales; no hace falta |
| **`GetDriveTypeW` + lista de permitidos `\Device\HarddiskVolume`** (elegida) | Falla cerrada. No depende del caso no verificado | Oculta las letras `subst` locales y cualquier volumen local con otro prefijo de dispositivo | Por defecto |

## Evidencia

`docs/spikes/results/2026-11-18/with-microsd.json` y `without-microsd.json`:

- `C:`: `drive_type 3` (`DRIVE_FIXED`), NTFS, `dos_device \Device\HarddiskVolume3`.
- microSD `D:`: `drive_type 2` (`DRIVE_REMOVABLE`), FAT32, `dos_device \Device\HarddiskVolume6`.
- Al retirar la microSD, `D:` desaparece del inventario; al reinsertarla, vuelve. Con eso basta re-enumerar sin caché.

**No verificado:** `subst X: \\servidor\compartida`, `net use` y symlink a UNC.
No existe `network-cases.json`. La decisión se diseñó para **no depender** de
ese resultado: con la regla 2, una letra cuyo destino no sea un
`\Device\HarddiskVolume` nunca se acepta, se reporte como se reporte. Las
letras de `subst` y `net use` dependen de la sesión y del token, así que la
comprobación final debe hacerse desde la tarea de ADR-004.

## Consecuencias

- La regla 2 lleva tests unitarios con destinos de ejemplo: los dos reales y los de `subst`, `LanmanRedirector`, `Mup` y `UNC`.
- Si alguien usa `subst` para acortar rutas locales, esas letras no aparecen. Se documenta en el README.
- La comprobación de red es una defensa en profundidad. La garantía de que nunca se toca la red la aporta `pathguard`, porque resuelve la ruta final por handle.

## Ajuste del ROADMAP

- **M1**: implementar la regla con sus tests.
- **M2** / `docs/manual-test.md`: ejecutar en la Ally, desde la tarea, el bloque de red de `spikes/volumes/README.md` (`subst` sobre UNC, `net use` y symlink a UNC). Hay que adjuntar `network-cases.json` y confirmar que ninguna de esas letras aparece en el panel.

## Condición de reapertura

- Un volumen local legítimo de la Ally, como una unidad USB o un VHD montado, aparece con otro prefijo de dispositivo y hay que mostrarlo.
- La evidencia de red muestra una letra de red con destino `\Device\HarddiskVolume`. Eso invalidaría la regla.
