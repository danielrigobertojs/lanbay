# ADR-001: decisiones del producto, no-objetivos y glosario

**Estado:** aceptado (2026-11-17)

## Contexto y decisión

Lanbay es un único binario Go para Windows 10/11 x64 que ofrece por HTTPS y
solo en LAN una interfaz web embebida. Administra archivos en volúmenes locales,
capacidad de disco y solo las aplicaciones instaladas por el propio panel.

Se mantiene un administrador, sin roles, SSO, acceso público ni recursos de
red. No incluye escritorio remoto, streaming, control de hardware, launchers,
Microsoft Store ni adopción de software preexistente. El frontend se entrega en
español y debe ser responsive para escritorio, teléfono y pantalla táctil.

## Glosario

- **Aplicación gestionada:** instalada por Lanbay y presente en su registro.
- **Volumen local:** disco fijo o removible físico; nunca un recurso de red.
- **Zona protegida:** ruta de solo lectura resuelta en el sistema objetivo.
- **Job:** trabajo persistente con progreso, log y cancelación.

## Consecuencias

Las decisiones marcadas `[VERIFICAR]` en el PRD requieren un spike y ADR antes
de que cualquier implementación dependa de ellas.
