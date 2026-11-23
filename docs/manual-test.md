# Pruebas manuales pendientes en Windows/Ally

## M1: migración segura de datos

Ejecutar como el usuario estándar antes de iniciar Lanbay elevado:

1. Crear `C:\ProgramData\Lanbay\junction-test` como junction hacia una carpeta de prueba controlada. No usar una ruta del sistema para esta comprobación.
2. Iniciar Lanbay con la tarea elevada o `lanbay run` elevado.
3. Confirmar que Lanbay se niega a arrancar, identifica una entrada insegura y que los permisos y el propietario de la carpeta destino no cambian.
4. Eliminar la junction. Verificar con `icacls C:\ProgramData\Lanbay` y `icacls C:\ProgramData\Lanbay\initial-password.txt` que el propietario es `BUILTIN\Administrators` y que solo Administrators y SYSTEM tienen control total.
5. Para comprobar una instalación anterior, crear como Administrador `C:\ProgramData\Lanbay` con una DACL heredada de `C:\ProgramData` (propietario `BUILTIN\Administrators`), iniciar Lanbay elevado y ejecutar `icacls C:\ProgramData\Lanbay`. Confirmar que la DACL pasa a estar protegida y contiene únicamente Administrators y SYSTEM con control total.

Esta comprobación requiere Windows real; no se declara verificada desde macOS.

## M2: borrado permanente en microSD FAT32

Antes del release, en la Ally con la microSD FAT32 montada como `D:`:

1. Crear un archivo de solo lectura y confirmar que Lanbay permite renombrarlo y borrarlo de forma permanente tras la confirmación.
2. Crear un archivo normal y borrarlo permanentemente. Confirmar que desaparece y que Lanbay no muestra una ventana del sistema.
3. Intentar borrar, copiar y comprimir una carpeta. Confirmar que la operación se completa a través del walker por handles (JCB-473 fusionado), que los reparse points se omiten con aviso en el log y que la carpeta resultante coincide.

Esta comprobación cubre los fallbacks `FileDispositionInfo`/`FileRenameInfo` para FAT32 y requiere Windows real; no se declara verificada desde macOS.

## M3: subida reanudable y suspensión

Pendiente de verificación en la Ally (E1); no se declara verificado desde macOS.

1. Desde un equipo de la LAN, subir una carpeta de 20 GB a la microSD usando el panel.
2. Durante un archivo, suspender la Ally y volver a despertarla. Confirmar que el navegador reconecta a `/api/v1/events` y que la carga puede continuar desde el offset mostrado.
3. Cancelar otra carga a mitad de archivo. Confirmar que no existe un archivo con el nombre final ni queda contenido parcial visible en el destino.
4. Medir el throughput visible en la red local; el objetivo es más de 80 MB/s si la red lo permite.
5. Con JCB-473 fusionado, probar también la subida de una carpeta completa (drag & drop) y una copia/movimiento interno de carpeta; verificar que la UI muestra progreso por archivo y total, velocidad y tiempo restante, y que no se produce escape de `pathguard`.

## E1: subida reanudable y suspensión

Pendiente de verificación en la Ally; no se declara verificado desde macOS.

1. Desde un equipo de la LAN, subir una carpeta de 20 GB a la microSD usando el panel.
2. Durante un archivo, suspender la Ally y volver a despertarla. Confirmar que el navegador reconecta a `/api/v1/events` y que la carga puede continuar desde el offset mostrado.
3. Cancelar otra carga a mitad de archivo. Confirmar que no existe un archivo con el nombre final ni queda contenido parcial visible en el destino.
4. Medir el throughput visible en la red local; el objetivo es más de 80 MB/s si la red lo permite.

## E2: análisis de uso por carpeta

Post-MVP (P1). Pendiente.

## E3: winget gestionado

Pendiente de verificación en la Ally; no se declara verificado desde macOS.

1. `winget search <término>` desde el panel devuelve resultados y respeta `--source winget`.
2. Instalar una app gestionada (p. ej. 7-Zip) desde el panel: aparece en el registro gestionado.
3. App preexistente (instalada fuera del panel) no aparece en la lista ni se puede instalar encima.
4. Actualizar y desinstalar una app gestionada; códigos 0/3010/1641 interpretados correctamente; banner de reinicio pendiente sin reiniciar.
5. Reconciliación al arrancar: app eliminada manualmente queda en estado `missing`.

## E4: instaladores subidos (MSI/EXE)

Pendiente de verificación en la Ally; no se declara verificado desde macOS.

1. Subir un `.msi` y un `.exe` (Inno Setup) a staging; verificar nombre, tamaño, SHA-256 y Authenticode.
2. Instalar el MSI en modo silencioso (`msiexec /i … /qn /norestart /l*v`): códigos 0/3010/1641.
3. Instalar el EXE con preset Inno en modo interactivo: ventana visible en pantalla (ADR-004 d).
4. Diff del registro de desinstalación: con 1 entrada se registra; con 0 = "no rastreable"; con >1 el usuario elige solo entre claves nuevas.
5. Desinstalar por `QuietUninstallString`/ProductCode; borrar instalador de staging tras éxito.
6. Verificar que un path fuera de staging se rechaza y que el directorio de staging se crea.

## E5: archivos, papelera y pathguard

Pendiente de verificación en la Ally; no se declara verificado desde macOS.

1. Listar, navegar, crear, renombrar, copiar, mover y borrar archivos en todos los volúmenes.
2. Borrado a la Papelera y restauración.
3. Descarga con Range y ZIP.
4. Casos adversariales de `pathguard` (relativas, `..`, UNC, prefijos de dispositivo, ADS, junctions).

## MicroSD: inserción y retirada

1. Insertar y retirar la microSD mientras el panel está en marcha.
2. Confirmar que los volúmenes se refrescan y que las operaciones en curso fallan de forma controlada sin corromper datos.

## Suspensión durante una subida

1. Iniciar una subida grande, suspender la Ally y reanudarla.
2. Confirmar que el navegador reconecta a `/api/v1/events` y que la subida continúa desde el offset mostrado.

## Junction hacia red

1. Crear una junction hacia `\\127.0.0.1\c$` dentro de un árbol y confirmar que Lanbay no la sigue (sin tráfico SMB) y la omite con aviso.
2. Verificar que el borrado no afecta al destino.

## Arranque tras reinicio

1. Reiniciar la Ally e iniciar sesión; confirmar que el panel responde en la LAN.
2. Suspender y reanudar; confirmar que el panel sigue respondiendo.
