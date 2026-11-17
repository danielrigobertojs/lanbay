# SPDX-License-Identifier: Apache-2.0

# Spike: Papelera con IFileOperation

Decide ADR-007: si la política por defecto puede ser reciclar también en la
microSD. `run.ps1` crea un archivo efímero por volumen y llama desde Go a un
script que usa `IFileOperation` con `FOFX_RECYCLEONDELETE`. No use archivos
reales como objetivos.

## Ejecución (PowerShell elevado)

```powershell
.\run.ps1 -Roots C:\,E:\
Get-Content .\out\recycle-bin.json
```

Cambie `E:\` por la letra de la microSD. Pegar el JSON y, para cada volumen,
indicar si el elemento apareció en la Papelera de Windows y si se pudo
restaurar. El informe incluye sistema de archivos y la presencia observable de
`$Recycle.Bin`; esa carpeta no es por sí sola una garantía de reciclaje.

Si la microSD elimina permanentemente el archivo o el API informa error, el
producto debe advertirlo antes de borrar y requerir confirmación de borrado
permanente. Limpie cualquier archivo `lanbay-m0-recycle-*` que quede tras un
fallo.
