# ADR-003: licencia y atribución

**Estado:** aceptado (2026-11-17)

Lanbay se distribuye bajo Apache-2.0, con Daniel Rigoberto Jacobo Sandoval como
titular. Se conserva el texto completo de Apache-2.0 y se cumplen las
condiciones §4(b), §4(c) y §4(d): cambios identificables, avisos de copyright,
marcas y atribución preservados, y `NOTICE` distribuido de forma legible.

La licencia no concede derechos sobre marcas (§6). Lanbay es el nombre propio
del proyecto. ASUS, ROG, Xbox, Microsoft, Windows, winget y
Microsoft.WinGet.Client se usan solo nominativamente; los dos últimos se
invocan, no se redistribuyen. No se usan esas marcas en identificadores,
binarios ni branding.

Se exige DCO y no CLA. Cada archivo Go, Svelte, TypeScript o JavaScript propio
lleva cabecera SPDX Apache-2.0. `scripts/gen-third-party.sh` genera las
atribuciones de módulos Go y paquetes npm que se incorporan al frontend; el
resultado se distribuye y `lanbay version` remite a él.
