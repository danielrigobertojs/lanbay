# SPDX-License-Identifier: Apache-2.0

# Spike: backend estructurado de winget

Decide ADR-005. Compare las tres alternativas **en el mismo usuario elevado**
del spike de contexto: módulo `Microsoft.WinGet.Client` en Windows PowerShell
5.1 y PowerShell 7, activación WinRT de `Microsoft.Management.Deployment` desde
Go y CLI. La CLI se mide solo como respaldo: su texto localizado no debe ser un
contrato de producción.

## Ejecución

Copie `probe.exe` y ejecute PowerShell elevado desde este directorio:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\run.ps1 -Query 7zip -Iterations 3
Get-Content .\out\winget-backend.json
```

El script no instala módulos ni paquetes. Si `Microsoft.WinGet.Client` o
PowerShell 7 faltan, el resultado lo registra como indisponible: no los instale
para ocultar esa condición. Capture además `winget --info` y `$PSVersionTable`.

## Qué pegar y decidir

Pegue `out/winget-backend.json` y ambos JSON de módulo. Compare latencia,
campos, errores y si el resultado cambia con la consola/configuración `es-MX`.
La activación WinRT desde Go prueba registro y disponibilidad del API; no afirma
que una búsqueda se haya ejecutado. La decisión debe preferir el módulo si es
estable en el contexto real, luego una implementación Go con ABI WinRT completa
validada posteriormente, y solo al final parseo CLI con fixtures de la Ally.
