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
