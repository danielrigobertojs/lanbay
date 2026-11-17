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

## Seguridad y licencia

No expongas Lanbay a internet. El binario aún no está firmado y SmartScreen o
Defender pueden advertirlo. Reporta vulnerabilidades en privado según
`SECURITY.md`. El proyecto usa Apache-2.0; consulta `LICENSE`, `NOTICE`,
`TRADEMARK.md` y `THIRD_PARTY_LICENSES.md`.
