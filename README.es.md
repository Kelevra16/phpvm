# phpvm

**Idiomas:** [English](README.md) · Español

[![CI](https://github.com/Kelevra16/phpvm/actions/workflows/ci.yml/badge.svg)](https://github.com/Kelevra16/phpvm/actions/workflows/ci.yml)
[![Release](https://github.com/Kelevra16/phpvm/actions/workflows/release.yml/badge.svg)](https://github.com/Kelevra16/phpvm/releases)
[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`phpvm` es un administrador de versiones y entornos PHP inspirado en [gobrew](https://github.com/kevincobain2000/gobrew). Instala distribuciones oficiales de PHP sin privilegios de administrador y proporciona un wrapper estable que no requiere reiniciar ni refrescar la terminal al cambiar de versión.

> **Candidato a lanzamiento:** actualmente el administrador de binarios funciona con las distribuciones oficiales de Windows x64/x86. Linux y macOS todavía no están soportados.

## Instalación

Instala o actualiza la versión más reciente desde PowerShell:

```powershell
irm https://raw.githubusercontent.com/Kelevra16/phpvm/main/install.ps1 | iex
```

Para instalar una versión específica:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/Kelevra16/phpvm/main/install.ps1))) -Version v0.2.0
```

El instalador:

- selecciona el artefacto correcto para Windows x64/x86;
- lo verifica contra el manifiesto SHA-256 de la release;
- instala `phpvm.exe` en `%LOCALAPPDATA%\phpvm\bin`;
- añade al `PATH` del usuario tanto `phpvm` como el directorio de wrappers de PHP administrado.

Abre una terminal nueva si es necesario y ejecuta:

```powershell
phpvm use 8.4
php --version
phpvm doctor
```

Para elegir otro destino o no modificar `PATH`, descarga el instalador y ejecuta:

```powershell
.\install.ps1 -InstallDir C:\Tools\phpvm -NoPathUpdate
```

Si todavía no existe una release, compila el proyecto desde el código fuente.

## Compilar desde el código fuente

```powershell
go test ./...
go build -o phpvm.exe ./cmd/phpvm
```

Se requiere Go 1.20 o posterior.

Coloca `phpvm.exe` en `PATH` y añade una sola vez `%USERPROFILE%\.phpvm\bin`. Usa `PHPVM_ROOT` para cambiar el directorio de almacenamiento predeterminado.

## Administración de versiones

```text
phpvm use latest                  instala y activa el PHP más reciente
phpvm use 8.4                     parche más reciente de la rama 8.4
phpvm use --ts 8.4                distribución thread-safe
phpvm use --arch x86 8.3          distribución de 32 bits
phpvm install 8.4                 instala sin activar
phpvm ls [--json]                 distribuciones instaladas
phpvm ls-remote [--ts] [--json]  distribuciones oficiales disponibles
phpvm current [--json]            distribución activa y metadatos
phpvm which [build]               ruta al php.exe seleccionado
phpvm uninstall <build>
phpvm prune                       conserva únicamente la distribución activa
```

Usa `--no-progress` en entornos no interactivos o `--quiet` para ocultar los mensajes de estado.

La identidad de una distribución incluye todas sus dimensiones de compatibilidad, por ejemplo `8.4.24-nts-x64`. Esto permite que TS/NTS o x64/x86 de la misma versión coexistan.

## Entornos de proyecto

Al ejecutar `phpvm` sin argumentos, busca desde el directorio actual hacia sus padres. El orden de resolución es:

1. `.php-version`
2. `phpvm.toml`
3. PHP de plataforma en `composer.lock`
4. PHP de plataforma o requisito en `composer.json`

`phpvm.toml` puede definir PHP y sus ajustes INI:

```toml
version = "8.4"
variant = "nts"
arch = "x64"

[ini]
memory_limit = "1G"
display_errors = "On"
```

Aplica el entorno completo del proyecto con:

```text
phpvm sync
```

Los proyectos pueden guardar sus errores PHP localmente:

```toml
[logs]
scope = "project"
path = ".phpvm/php-error.log"
```

Las restricciones comunes de Composer se resuelven contra las ramas oficiales disponibles, incluyendo `^8.3`, `~8.3.2`, `>=8.2 <8.5`, `8.4.*` y alternativas con `||`. Todavía no se soportan indicadores de estabilidad, rangos con guion ni todos los casos extremos del solucionador completo de Composer.

## Configuración, perfiles y extensiones

```text
phpvm ini get memory_limit
phpvm ini set memory_limit 1G
phpvm ini path
phpvm ini show
phpvm ini diff
phpvm ini reset

phpvm profile create laravel
phpvm profile set laravel memory_limit 1G
phpvm profile set laravel display_errors On
phpvm profile use laravel

phpvm ext ls
phpvm ext enable curl
phpvm ext disable curl
```

La administración de extensiones actualmente habilita o deshabilita DLL incluidas en la distribución oficial de PHP. La descarga y resolución de paquetes PECL externos será responsabilidad de un proveedor futuro.

## Logs de errores

El registro de errores PHP se configura automáticamente para la distribución activa. La ubicación predeterminada es `~/.phpvm/logs/<build>/php-error.log`; `phpvm.toml` puede sobrescribirla por proyecto.

```text
phpvm logs path
phpvm logs show
phpvm logs show --lines 200
phpvm logs tail --lines 50
phpvm logs tail --follow
phpvm logs open
phpvm logs doctor
phpvm logs clear --force
```

`logs open` usa la aplicación predeterminada del sistema. `tail --follow` continúa hasta interrumpirse con Ctrl+C. Limpiar un log requiere `--force` para evitar que un script lo borre accidentalmente.

## Comandos reproducibles y matrices

Ejecuta un comando con una versión concreta sin cambiar la distribución global:

```text
phpvm exec 8.3 -- php --version
phpvm exec -- composer test
```

Ejecuta el mismo comando contra varias ramas PHP:

```text
phpvm matrix 8.2 8.3 8.4 -- php vendor/bin/phpunit
```

## Proyectos simultáneos y terminales aisladas

Abre sesiones PowerShell independientes para proyectos que requieren versiones distintas:

```powershell
# Terminal A
cd C:\proyectos\aplicacion-legada
phpvm shell 7.4
php --version

# Terminal B
cd C:\proyectos\aplicacion-moderna
phpvm shell 8.5
php --version
```

La terminal hija establece `PHPVM_ACTIVE` y antepone únicamente el directorio PHP seleccionado a `PATH`. Escribe `exit` para regresar a la terminal padre. La versión global no cambia.

```text
phpvm shell 8.4        instala si hace falta y abre una terminal aislada
phpvm shell            resuelve el proyecto actual y abre una terminal
phpvm shell --current  usa la distribución global en una terminal aislada
```

El wrapper estable `php.cmd` también resuelve una distribución en cada ejecución. Esto permite usar versiones distintas desde varios directorios al mismo tiempo. La prioridad es:

```text
PHPVM_ACTIVE → configuración del proyecto → distribución global activa
```

La configuración del proyecto puede provenir de `.php-version`, `phpvm.toml` o una restricción Composer soportada. El shim nunca descarga PHP implícitamente; si falta una distribución solicita ejecutar `phpvm sync`.

Inspecciona la distribución o ejecutable seleccionado sin ejecutar PHP:

```text
phpvm resolve
phpvm resolve --path
phpvm resolve 8.4
```

Los alias se almacenan independientemente de las distribuciones instaladas:

```text
phpvm alias set legacy 7.4
phpvm alias set stable 8.4
phpvm use stable
phpvm alias ls
phpvm alias remove legacy
```

## Mantenimiento cotidiano

Localiza el ejecutable activo, inspecciona la caché o actualiza `phpvm`:

```text
phpvm which
phpvm which 8.3
phpvm cache dir
phpvm cache clear
phpvm self-update
phpvm self-update v0.2.0
```

`self-update` descarga la GitHub Release seleccionada, verifica su checksum SHA-256 publicado, prepara el ejecutable nuevo y sustituye el binario en ejecución cuando termina el comando.

## Autocompletado de PowerShell

Genera el completador nativo de argumentos:

```powershell
New-Item -ItemType Directory -Force (Split-Path $PROFILE) | Out-Null
phpvm completion powershell | Add-Content $PROFILE
. $PROFILE
```

El autocompletado cubre comandos, subcomandos y distribuciones instaladas para las operaciones comunes.

## Laragon

`phpvm` detecta Laragon mediante `LARAGON_ROOT`, `C:\laragon` o `C:\tools\laragon`:

```text
phpvm laragon detect
phpvm laragon link
phpvm laragon link 8.3
phpvm laragon unlink 8.3
```

`link` crea una unión de directorios dentro de `bin\php` de Laragon. Selecciona la entrada `phpvm-<build>` en el menú de versiones PHP de Laragon y recarga Laragon. Eliminar la unión no elimina la distribución PHP administrada.

## Integridad y diagnóstico

```text
phpvm doctor [--json]
phpvm verify [build]
phpvm repair [build]
phpvm clean
```

Las instalaciones son transaccionales:

1. Un lock entre procesos serializa las modificaciones.
2. El archivo se descarga a una ubicación temporal.
3. Se verifica su checksum SHA-256 oficial.
4. Se extrae en un directorio de preparación con protección contra ZIP traversal.
5. Se valida y calcula el hash de `php.exe`.
6. El directorio terminado se publica atómicamente.

Cada distribución contiene `phpvm.json` con versión, variante, arquitectura, URL de origen, checksums y fecha de instalación. `verify` detecta cambios posteriores en `php.exe`; `repair` sustituye una distribución desde su fuente oficial registrada preservando su identidad activa.

El registro oficial de releases se almacena en caché durante seis horas. Configura `PHPVM_CACHE_TTL` con una duración Go como `30m` o `24h`. Usa `PHPVM_TIMEOUT`, por ejemplo `PHPVM_TIMEOUT=10m`, para limitar la duración de un comando.

Códigos de salida: `0` éxito, `1` fallo operativo, `2` uso inválido y `124` timeout. Los comandos hijos ejecutados mediante `phpvm exec` conservan su propio código distinto de cero.

## Desinstalación

Ejecuta el script de desinstalación del repositorio para eliminar el ejecutable y conservar las versiones PHP instaladas:

```powershell
.\uninstall.ps1
```

Usa `-RemoveData` únicamente para eliminar también todas las versiones PHP, perfiles, logs y configuración administrados.

## Solución de problemas de instalación

Si `phpvm version` muestra `dev` después de instalar una release, inspecciona todos los ejecutables coincidentes:

```powershell
Get-Command phpvm -All
where.exe phpvm
```

La ubicación canónica es `%LOCALAPPDATA%\phpvm\bin\phpvm.exe`. Los instaladores actuales renombran un ejecutable anterior en `~\.phpvm\bin\phpvm.exe` como `phpvm.exe.legacy` y colocan primero el directorio canónico en `PATH`. Abre una terminal nueva después de instalar para que PowerShell lea el entorno actualizado.

## Estructura de almacenamiento

```text
~/.phpvm/
  aliases.json
  profiles.json
  current
  bin/php.cmd
  versions/
    8.4.24-nts-x64/
      php.exe
      php.ini
      phpvm.json
```

## Roadmap pendiente

- proveedores para Linux y macOS;
- resolución completa de restricciones Composer;
- instalación de extensiones PECL externas y resolución de dependencias;
- selectores de prereleases y metadatos remotos en caché;
- autocompletado para Bash y Zsh;
- artefactos firmados;
- automatización más completa de Laragon y recarga automática;
- hooks opcionales del ciclo de vida.
