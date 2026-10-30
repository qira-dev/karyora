# Karyora — Guía del usuario

Esta guía describe la aplicación de escritorio Karyora tal como se distribuye hoy. Solo declara lo que el producto realmente hace; las capacidades que aún no existen se enumeran como limitaciones y nunca se presentan como promesas. Esta misma guía está disponible en 12 idiomas en esta carpeta.

## 1. Qué es Karyora

- Karyora es una aplicación de escritorio con prioridad local (local-first). Tus paquetes de conocimiento, trabajos y ajustes se guardan en tu propia máquina.
- El producto funciona sin ningún servicio en la nube del proyecto. El único tráfico de red que puede generar es el que tú inicias: solicitudes a tus proveedores de IA, una importación de paquete de conocimiento que tú solicites y — si la configuras — el canal de actualizaciones.
- Las funciones de IA usan tus propias cuentas de proveedor (BYOK, "bring your own key"). Tú configuras tus propias claves de API y el proveedor te cobra directamente. Karyora nunca revende el uso de los proveedores ni actúa como intermediaria de sus pagos.

## 2. Versiones comunitarias y oficiales

- El código fuente de Karyora se publica bajo la licencia Apache License 2.0. Puedes usarlo, estudiarlo, modificarlo y redistribuirlo conforme a esa licencia, incluida su compilación por tu cuenta.
- El repositorio público es `https://github.com/qira-dev/karyora`. Cada versión se marca con una etiqueta `v*`, y el árbol fuente de esa etiqueta es el código fuente de la versión.
- Una versión "oficial" es la que construyó el propio pipeline de compilación del proyecto en una revisión etiquetada, con evidencia registrada. Las compilaciones oficiales y las comunitarias tienen las mismas funciones; la diferencia es quién construyó el artefacto y quién responde por él, nunca qué puede hacer el artefacto.
- En la v1 todavía no existe una página oficial de descargas ni un canal oficial de actualizaciones. Hasta que se publiquen, puedes obtener Karyora desde el repositorio público y compilarlo desde el código fuente.

## 3. Plataformas compatibles (v1)

| SO | CPU | Mínimo |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 o más reciente |
| Windows 11 | ARM64 | Windows 11 para ARM |
| macOS | x86_64 | macOS 15 (Sequoia) o más reciente |
| macOS | ARM64 | macOS 15 (Sequoia) o más reciente, Apple Silicon |
| Linux | x86_64 | glibc 2.34 o más reciente; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 o más reciente; las mismas familias de distribuciones |

No compatibles en la v1 de forma explícita: Windows 10; macOS 14 o anterior; distribuciones basadas en musl como Alpine; cualquier combinación no listada arriba. El soporte sigue la ventana de seguridad de cada proveedor y se vuelve a comprobar en cada versión.

## 4. Paquetes y cómo verificarlos

- El conjunto de paquetes producidos es: MSI de Windows, pkg de macOS, deb de Linux, rpm de Linux y un archivo portátil de Linux — un artefacto por cada arquitectura de CPU compatible.
- Cada paquete se genera junto con un manifiesto `SHA256SUMS` y registros de SBOM y procedencia por artefacto. Para verificar un paquete: calcula el resumen sha256 de tu descarga y compáralo con la entrada del manifiesto. Si el resumen no coincide, no instales el paquete.
- Los paquetes de la v1 no están firmados digitalmente — por diseño. Los pasos de verificación de firmas de la plataforma (comprobaciones de firma del sistema operativo, claves de repositorio) se documentarán junto con la primera versión firmada. Hasta entonces, la comparación de resúmenes de arriba es la verificación que puedes realizar.
- Los archivos `LICENSE` y `NOTICE`, que contienen la licencia del proyecto y las atribuciones de terceros, forman parte del repositorio fuente.

## 5. Actualizaciones

- Karyora incluye un mecanismo de actualización, y en la v1 se distribuye desactivado de forma predeterminada. No hay canal de actualizaciones configurado, por lo que una comprobación de actualizaciones informa de un rechazo tipificado que indica que no hay endpoint configurado. Esto es intencional: el canal oficial de actualizaciones solo se activará cuando se publique.
- Hasta entonces, actualizar significa instalar el paquete más nuevo sobre el anterior. Tus datos se conservan durante las actualizaciones (véase la sección 6).

## 6. Tus datos

- Todos los datos del producto viven en tu máquina. El directorio de datos predeterminado es `%APPDATA%\Karyora\data` en Windows, `~/Library/Application Support/Karyora/data` en macOS y `~/.local/share/karyora/data` en Linux (se respeta la opción XDG `XDG_DATA_HOME`). Puedes elegir otra ubicación con la opción `--data-dir`.
- El producto no realiza copias de seguridad programadas. Para hacer una copia, detén el producto y copia el directorio de datos. Restaurar consiste en volver a colocar el directorio copiado en su sitio. No hay copia de seguridad en la nube.
- Desinstalar o actualizar no borra tus datos. Borrar datos es una acción aparte y explícita que tú realizas.
- Los datos en reposo no están cifrados. Protege el directorio de datos con tus propios medios, como el cifrado de disco completo. Es una limitación conocida, declarada abiertamente.
- Las credenciales de los proveedores (tus claves de API) se guardan únicamente en el llavero de tu sistema operativo. Nunca se escriben en archivos, registros ni en el contenido de las solicitudes.

## 7. Privacidad

- El producto no incluye telemetría: no se envía nada a los servidores del proyecto.
- Los diagnósticos se generan localmente y por defecto respetan la privacidad; tú decides si algo sale de tu máquina o no.

## 8. Limitaciones conocidas de la v1

- La ejecución de plugins de terceros está desactivada.
- La operación en equipo y remota no está disponible.
- Transferir un trabajo activo de una máquina a otra no está admitido.
- La sincronización de paquetes de conocimiento portátiles no está disponible.
- La función de agrupación "Work" no existe.
- No hay suscripciones de pago ni derechos (entitlements): nada que comprar, sin niveles de pago.
- No se hacen promesas numéricas de rendimiento.
- Los datos en reposo no están cifrados (véase la sección 6).
- Todavía no hay página oficial de descargas ni canal de actualizaciones (véanse las secciones 2 y 5).

## 9. Licencia

- Karyora se publica bajo la licencia Apache License 2.0. El texto completo de la licencia está en el archivo `LICENSE` y las atribuciones de terceros en el archivo `NOTICE`, ambos en el repositorio fuente.
- Compilar Karyora por tu cuenta, crear un fork o redistribuirlo conforme a la licencia conserva todos los derechos que la licencia otorga. Este derecho no depende de ninguna compra, porque no hay nada que comprar.

## 10. Referencia de la línea de comandos

La herramienta de línea de comandos es `uawp`.

| Comando | Qué hace |
|---|---|
| `uawp start` | Inicia el servicio local. Un segundo arranque sobre el mismo directorio de datos se rechaza mientras ya haya uno en ejecución. |
| `uawp stop` | Detiene el servicio con un apagado limitado y ordenado. |
| `uawp status` | Informa si el servicio está en ejecución. |
| `uawp doctor` | Ejecuta comprobaciones locales de salud y diagnóstico. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Comprueba, aplica o informa sobre actualizaciones. En la v1, sin canal configurado, check y apply terminan con un rechazo tipificado que indica que no hay endpoint configurado; `status` informa del estado de actualización. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Envía un trabajo duradero, informa su estado o sigue sus eventos. |
| `uawp credential store --provider <id> [--name api-key]` | Guarda tu clave de API del proveedor en el llavero del sistema operativo. |
| `uawp credential status` | Informa si existe la credencial — nunca su valor. |
| `uawp credential remove` | Elimina la credencial guardada. |
| `uawp version` / `uawp help` | Muestra la versión o la ayuda. |

Una credencial almacenada por sí sola no abre nada: una llamada real al proveedor todavía debe pasar las reglas de manejo de datos que el producto aplica a ese proveedor.
