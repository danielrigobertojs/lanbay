# ADR-004: contexto de ejecución

**Estado:** aceptado con condiciones (2026-11-18). Revisión: al cerrar M5 y M6.

## Contexto

Lanbay necesita privilegios elevados para escribir en casi cualquier ruta e
instalar software. winget es una app por usuario, y el modo interactivo de los
`.exe` necesita el escritorio del usuario (PRD §7.1, §5.5.3). El PRD propone
una tarea programada al iniciar sesión con "privilegios más altos" y deja como
alternativa un servicio SYSTEM más un agente de usuario.

## Decisión

Lanbay corre como **tarea programada del usuario**, con disparador al iniciar
sesión, `LogonType Interactive`, `RunLevel Highest` y oculta. No hay servicio
ni proceso SYSTEM. `lanbay install` registra la tarea con la misma forma que
`spikes/execution-context/register-task.ps1`.

## Alternativas

| Opción | Ventajas | Desventajas | Cuándo conviene |
|---|---|---|---|
| **Tarea elevada en la sesión del usuario** (elegida) | Un proceso. winget, el escritorio y las letras del usuario están disponibles. Sin UAC en cada arranque | No hay panel antes del inicio de sesión. El proceso tiene el token de administrador del usuario | Consola personal con inicio de sesión automático |
| Servicio SYSTEM + agente de usuario | Panel disponible antes del inicio de sesión. Separa privilegios | IPC autenticado, dos binarios o modos, dos ciclos de vida, más superficie. winget como SYSTEM es problemático (PRD §7.1) | Solo si la tarea no puede instalar o mostrar UI |
| Proceso sin elevar en la sesión | Menor privilegio | No instala software ni escribe en rutas protegidas sin UAC por operación | No cumple los requisitos del producto |

## Evidencia

`docs/spikes/results/2026-11-18/execution-context/execution-context.json`,
producido por la tarea `Lanbay-M0-ExecutionContext` lanzada con
`Start-ScheduledTask`:

- Token elevado: `Nivel obligatorio alto` (S-1-16-12288), `BUILTIN\Administradores` habilitado, grupo `INTERACTIVE` y `INICIO DE SESIÓN EN LA CONSOLA`.
- (a) `winget search` y `winget install 7zip.7zip --silent ... --disable-interactivity`: `exit_code 0` (6,6 s).
- (c) Escritura y borrado en `C:\` y en la microSD `D:\`: `exit_code 0`.

**No verificado:**

- (b) MSI silencioso: no se ejecutó.
- (d) Instalador interactivo visible en el escritorio: no se ejecutó.
- El disparador al iniciar sesión: la tarea se lanzó a mano, no en un inicio de sesión real.
- La reanudación tras suspender la consola.

## Consecuencias

- Se descarta el servicio SYSTEM mientras no se cumpla una condición de reapertura. No hay IPC en M1–M6.
- El panel no responde antes del inicio de sesión. Es aceptable (PRD §7.1) y se documenta en el README.
- El proceso tiene privilegios de administrador y escucha en la LAN. El modelo de amenazas (JCB-459) lo trata como activo crítico: un fallo de auth o de `pathguard` equivale a control del equipo.
- La tarea va oculta. Un instalador interactivo hereda la sesión, pero no hay evidencia de que su ventana aparezca en pantalla.

## Ajuste del ROADMAP

- **M5** no se cierra sin evidencia en la Ally de (b) MSI silencioso y (d) EXE interactivo visible, ambos lanzados desde la tarea. Si (d) falla, el modo interactivo sale del MVP y la UI lo indica. No se adopta el servicio por ese único motivo.
- **M6** verifica en la Ally que la tarea arranca sola tras reiniciar e iniciar sesión, y que el panel sigue respondiendo tras suspender y reanudar.

## Condición de reapertura

Reabrir y evaluar el servicio SYSTEM + agente de usuario si ocurre alguna de estas cosas:

- El MSI silencioso falla desde la tarea por una causa atribuible al contexto.
- El disparador de inicio de sesión no arranca el panel de forma fiable.
- El producto pasa a requerir disponibilidad antes del inicio de sesión.
