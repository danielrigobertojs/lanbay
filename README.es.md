# Lanbay

Lanbay es un panel web HTTPS local para Windows 10/11 x64 que administra el
almacenamiento local y únicamente el software instalado por el propio panel.
Es un binario Go único: la interfaz Svelte se compila y queda embebida, sin
CDN ni servicios externos en tiempo de ejecución.

El repositorio está en su etapa de fundación. Toda función exclusiva de Windows
queda pendiente de verificación en el equipo objetivo; macOS no la valida.

## Compilar

Se requieren Go estable, Node.js/npm y `staticcheck`.

```text
npm --prefix web ci
make windows
dist/lanbay.exe version
```

El objetivo `windows` usa `CGO_ENABLED=0` y genera `windows/amd64`.

## Kit de bootstrap M0 para Windows

Antes de instalar Lanbay se deben ejecutar sus spikes M0 en el equipo Windows
objetivo. Desde una Mac en la misma LAN privada, genera y sirve un kit ya
compilado:

```sh
make ally-kit
go run ./tools/kitserver
```

El servidor imprime un único comando. Abre **PowerShell como administrador**
en Windows y pégalo. El servidor es temporal, genera un token aleatorio, solo
atiende clientes LAN privados o link-local, y se detiene al recibir los
resultados o tras 30 minutos de inactividad. El bootstrap descarga el kit,
verifica su SHA-256 antes de extraer o ejecutar nada y guía los pasos que
requieren presencia: microSD, MSI de prueba y confirmaciones de acciones
destructivas. Al final sube un archivo de resultados sin modificar a
`m0-results/<timestamp>/` en la Mac.

Este flujo es una comodidad exclusiva de M0; no instala Lanbay. Usa HTTP de
forma deliberada porque el propio comando de bootstrap sale de la Mac local.
El token y el hash fijado del kit reducen el riesgo, pero no ofrecen la
protección TLS frente a un atacante que controle la LAN. Es un riesgo aceptado
y limitado en el tiempo para M0: no uses este servidor para distribuir
releases de Lanbay.

Consulta la [guía de spikes M0](spikes/README.md) para prerrequisitos, la
alternativa manual, limpieza y la evidencia que aún debe obtenerse en Windows.

## Seguridad y licencia

No expongas Lanbay a internet. El binario aún no está firmado y SmartScreen o
Defender pueden advertirlo. Reporta vulnerabilidades en privado según
`SECURITY.md`. El proyecto usa Apache-2.0; consulta `LICENSE`, `NOTICE`,
`TRADEMARK.md` y `THIRD_PARTY_LICENSES.md`.
