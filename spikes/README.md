# SPDX-License-Identifier: Apache-2.0

# Spikes M0 para Windows

Estos artefactos se ejecutan únicamente en el equipo Windows objetivo. No son
parte de `lanbay.exe`, ni constituyen una instalación del producto. Cada
subdirectorio produce un informe JSON en `out/`; adjunte los cuatro informes
sin modificar al ticket de ejecución M0 junto con versión de Windows, idioma,
versión de App Installer/winget, PowerShell y formato de la microSD.

## Flujo principal: kit servido desde la Mac

En la Mac, desde la raíz del repositorio, genere el kit y arranque un servidor
efímero:

```sh
make ally-kit
go run ./tools/kitserver
```

El servidor queda ligado únicamente a una IP privada o link-local de la Mac,
genera un token aleatorio por ejecución e imprime la única línea que debe
pegarse en **PowerShell abierto como Administrador** en Windows. El bootstrap
descarga el zip, comprueba su SHA-256 antes de extraer o ejecutar nada, guía
las acciones que requieren presencia (microSD, MSI y confirmaciones), y sube
un zip de resultados a `m0-results/<timestamp>/`. El servidor se apaga tras
recibirlos o después de 30 minutos sin actividad; ambos límites se pueden
cambiar con `-idle-timeout` y `-max-result-bytes`.

El flujo usa HTTP deliberadamente y solo para M0. El token de vida corta y el
hash SHA-256 fijado en el bootstrap reducen la exposición, pero no sustituyen
TLS: un atacante con control de la LAN podría sustituir el bootstrap antes de
que se conozca el hash. Es un riesgo aceptado exclusivamente para estos spikes;
no reutilice este servidor para instalar Lanbay.

Para una comprobación no interactiva de CI, el bootstrap admite `-NonInteractive`:
solo descarga, verifica, extrae, ejecuta el spike de volúmenes y sube su JSON.

## Alternativa manual

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
