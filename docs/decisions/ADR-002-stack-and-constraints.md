# ADR-002: stack y restricciones

**Estado:** aceptado (2026-11-17)

Lanbay usa Go estable con `CGO_ENABLED=0`, `net/http` (routing de Go 1.22+),
`modernc.org/sqlite` y `golang.org/x/sys/windows`. El frontend es Svelte +
Vite, compilado a estáticos y embebido mediante `embed.FS`; no hay CDNs.

El código específico del SO queda detrás de interfaces y archivos con build
tags `_windows.go` y `_unix.go`; no se dispersa `runtime.GOOS` en la lógica de
negocio. macOS y Linux son plataformas de desarrollo, con software deshabilitado.

Dependencias iniciales cerradas: `modernc.org/sqlite`, `golang.org/x/sys`,
Svelte, el plugin de Vite para Svelte y Vite. Una nueva dependencia requiere
justificación en el ticket y README. El binario se llama `lanbay.exe` y sus
datos irán en `%ProgramData%\\Lanbay\\`.
