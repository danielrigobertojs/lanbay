# SPDX-License-Identifier: Apache-2.0

# Spike: volúmenes locales y exclusión de red

Decide ADR-006: qué señales permiten excluir una letra que parece local pero
termina en red. `probe.exe` enumera con `GetLogicalDrives`, `GetDriveTypeW`,
`GetVolumeInformationW` y `QueryDosDeviceW`; no modifica discos.

## Ejecución

Ejecute y guarde una línea de base con la microSD insertada, luego repita tras
retirarla de forma segura:

```powershell
.\probe.exe -out .\out\with-microsd.json
# retirar la microSD de forma segura
.\probe.exe -out .\out\without-microsd.json
```

Para los casos de red, use una compartición de pruebas a la que tenga acceso:

```powershell
subst X: \\servidor\compartida       # puede fallar: pegue exactamente el error
net use Y: \\servidor\compartida
cmd /c mklink /D C:\LanbayM0-UncLink \\servidor\compartida
.\probe.exe -out .\out\network-cases.json -reparse-path C:\LanbayM0-UncLink
subst X: /d; net use Y: /delete; Remove-Item C:\LanbayM0-UncLink -Force
```

Pegar los tres JSON y el texto de cualquier fallo. El resultado debe mostrar si
`subst` acepta UNC y cómo lo expone el dispositivo DOS; una letra `DRIVE_FIXED`
no basta para aceptarla si apunta a UNC. La prueba de junction/symlink es
exploratoria: Windows puede rechazar una junction hacia UNC; eso también es
evidencia útil.
