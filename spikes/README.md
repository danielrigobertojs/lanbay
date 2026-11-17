# SPDX-License-Identifier: Apache-2.0

# Spikes M0 para Windows

Estos artefactos se ejecutan únicamente en el equipo Windows objetivo. No son
parte de `lanbay.exe`, ni constituyen una instalación del producto. Cada
subdirectorio produce un informe JSON en `out/`; adjunte los cuatro informes
sin modificar al ticket de ejecución M0 junto con versión de Windows, idioma,
versión de App Installer/winget, PowerShell y formato de la microSD.

Desde la raíz del repositorio, compile antes de copiarlos al equipo:

```powershell
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build ./spikes/...
```

Los README individuales contienen pasos, efectos y limpieza. Los comandos que
instalan o borran algo exigen una opción explícita. No ejecute un spike mientras
Windows Update u otro instalador esté activo.
