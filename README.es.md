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

## Instalación (Windows)

Ejecuta desde PowerShell o Command Prompt elevado:

```text
lanbay.exe install
```

`install` crea `%ProgramData%\Lanbay` con una ACL solo para Administrators y
SYSTEM, registra la tarea programada `Lanbay` (al iniciar sesión, interactiva,
con privilegios máximos y oculta) y añade una regla de firewall entrante para
el puerto HTTPS configurado **solo en el perfil Privado**. También genera la
contraseña inicial del administrador y la escribe en
`%ProgramData%\Lanbay\initial-password.txt` (protegido). Cámbiala tras el
primer inicio de sesión.

Lanbay arranca solo tras iniciar sesión; no hay panel antes del inicio de
sesión. Las letras de unidad creadas con `subst` no se muestran (ver ADR-006).

Para desinstalar:

```text
lanbay.exe uninstall
```

`uninstall` elimina la tarea programada y la regla de firewall, y pregunta si se
conserva el directorio de datos. Las aplicaciones gestionadas instaladas por el
panel **no** se desinstalan.

## Confianza en el certificado

Lanbay usa un certificado TLS autofirmado. Su huella SHA-256 se imprime al
arrancar y se muestra en el panel. Para evitar avisos del navegador:

- **macOS:** importa `tls-cert.pem` en el llavero del Sistema y márcalo como
  "Confiar siempre" para SSL.
- **iOS/iPadOS:** instala el perfil y activa la confianza completa en
  Ajustes > General > Información > Ajustes de confianza de certificados.
- **Windows (cliente):** impórtalo en "Entidades de certificación raíz de
  confianza".

## SmartScreen / Defender de Windows

El binario no está firmado. SmartScreen y Defender pueden advertir sobre un
publicador desconocido; elige "Más información" > "Ejecutar de todas formas"
solo tras verificar el checksum SHA-256 de la release. Ver `SECURITY.md`.

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

## Dependencias

- `modernc.org/sqlite`: driver SQLite en Go puro.
- `golang.org/x/sys/windows`: bindings de la API de Windows.
- `golang.org/x/crypto/argon2`: hash de contraseña del administrador resistente a memoria.
- Svelte: interfaz web compilada localmente.

La atribución completa generada está en `THIRD_PARTY_LICENSES.md` y se nombra en
`lanbay version`.

## Seguridad y licencia

No expongas Lanbay a internet. El binario aún no está firmado y SmartScreen o
Defender pueden advertirlo. Reporta vulnerabilidades en privado según
`SECURITY.md`. El proyecto usa Apache-2.0; consulta `LICENSE`, `NOTICE`,
`TRADEMARK.md` y `THIRD_PARTY_LICENSES.md`.
