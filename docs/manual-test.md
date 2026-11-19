# Pruebas manuales pendientes en Windows/Ally

## M1: migración segura de datos

Ejecutar como el usuario estándar antes de iniciar Lanbay elevado:

1. Crear `C:\ProgramData\Lanbay\junction-test` como junction hacia una carpeta de prueba controlada. No usar una ruta del sistema para esta comprobación.
2. Iniciar Lanbay con la tarea elevada o `lanbay run` elevado.
3. Confirmar que Lanbay se niega a arrancar, identifica una entrada insegura y que los permisos y el propietario de la carpeta destino no cambian.
4. Eliminar la junction. Verificar con `icacls C:\ProgramData\Lanbay` y `icacls C:\ProgramData\Lanbay\initial-password.txt` que el propietario es `BUILTIN\Administrators` y que solo Administrators y SYSTEM tienen control total.

Esta comprobación requiere Windows real; no se declara verificada desde macOS.
