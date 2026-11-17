# SPDX-License-Identifier: Apache-2.0

# Spike: contexto de ejecución elevado en sesión de usuario

Decide ADR-004: si una tarea al iniciar sesión, elevada y oculta puede ser el
host único de Lanbay. El script registra `Lanbay-M0-ExecutionContext` con
`LogonTrigger`, `RunLevel=Highest` y tarea oculta; no usa SYSTEM.

## Preparación

1. Compile `runner.exe` con el comando de `spikes/README.md` y copie este
   directorio a una ruta local, por ejemplo `C:\LanbayM0\execution-context`.
2. Inserte la microSD y anote su letra. Descargue por separado un MSI de prueba
   permitido por su licencia; no se incluye ninguno aquí. Para una prueba
   destructiva reproducible use una VM/snapshot o un MSI que se pueda
   desinstalar después.
3. Para probar el modo interactivo, indique un EXE que pueda mostrarse con
   seguridad. El runner solo lo inicia: la persona debe confirmar que apareció
   en el escritorio y cerrarlo.

## Ejecución (PowerShell elevado)

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\register-task.ps1 -RunnerPath C:\LanbayM0\execution-context\runner.exe `
  -MicroSDRoot E:\ -TestMSI C:\Users\me\Downloads\test.msi `
  -InteractiveInstaller C:\Users\me\Downloads\setup.exe
Start-ScheduledTask -TaskName Lanbay-M0-ExecutionContext
Start-Sleep -Seconds 20
Get-Content .\out\execution-context.json
```

Pegar: `out/execution-context.json`, una captura donde se vea el instalador
interactivo y `Get-ScheduledTask ... | Format-List *`. El informe registra
identidad, elevación, sesión, búsqueda/instalación winget, MSI y escritura
efímera en `C:\` y microSD. El paquete winget por defecto es `7zip.7zip`; si
ya existe, el informe lo dice. Cambie `-WingetID` por un paquete de prueba que
pueda eliminarse. La instalación winget requiere `-AllowInstall`; el MSI
requiere además `-AllowMSI` y el EXE interactivo `-AllowInteractive`. El
bootstrap pide una confirmación explícita antes de pasar esos permisos.

## Limpieza

```powershell
.\cleanup.ps1 -WingetID 7zip.7zip
```

Revise primero que no necesitaba el paquete. El MSI y EXE no se desinstalan
automáticamente: documente y ejecute su propio desinstalador si correspondía.

## Decisión

Aceptar la propuesta solo si las cuatro comprobaciones pasan y el instalador
interactivo se ve en el escritorio. Si winget o la UI no funcionan, no adoptar
un servicio SYSTEM de forma implícita: documentar el fallo y evaluar el diseño
servicio + agente de usuario.
