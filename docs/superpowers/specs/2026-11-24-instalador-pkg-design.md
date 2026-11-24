# Diseño — Instalador redistribuible (.pkg) del agente de impresión

Fecha: 2026-11-24
Estado: pendiente de aprobación

## Intención y por qué

Hoy el repo `isphub-print-agent` distribuye un binario crudo (`isphub-print-agent-darwin-arm64`)
y deja la instalación en manos del usuario (`chmod`, `xattr -d`, `discover`, `config`, `install`).
El mostrador de un ISP no debería hacer eso.

Queremos un **instalador redistribuible** que sea EL release: un `.pkg` nativo de macOS que, al
doble clic, instale el agente, lo deje arrancando al iniciar sesión, y abra un asistente gráfico
que detecte la impresora automáticamente (y pida la IP a mano solo si no la encuentra).

El "porqué" es operativo: bajar la fricción de instalación para que el personal del mostrador
ponga a imprimir recibos en minutos, sin terminal, sin Node, sin drivers.

## Qué se decidió (con el dueño del producto)

1. **Formato:** `.pkg` (Instalador.app nativo de macOS), doble clic → Instalar → listo.
2. **Asistente de configuración:** página web servida por el propio agente (no una app nativa).
   Flujo: detecta la IP automáticamente; si no encuentra la impresora, pide la IP manual.
3. **Firma:** firmado y notarizado con **Developer ID** (Application + Installer). El dueño
   creará los certificados y la API key de notarización; no se publica sin notarizar.

## Metas

- Un `.pkg` firmado y notarizado que se descarga de GitHub Releases y se instala con doble clic.
- Tras instalar: el agente corre como LaunchAgent del usuario, escuchando en `127.0.0.1:8787`.
- Se abre el navegador en `http://127.0.0.1:8787/setup` con el asistente de configuración.
- El asistente detecta la EC-PM-80330 sola; si no la ve, permite escribir la IP.
- El asistente guarda la IP y hace una impresión de prueba.

## No-metas

- No app nativa SwiftUI (descartada en favor de la página web).
- No soporte Mac Intel (el binario es arm64; igual que hoy).
- No firma/notarización de app de App Store (es distribución directa, no MAS).
- No cambiar el contrato de impresión ni el protocolo ESC/POS.
- No incluir drivers: la impresora es de red (TCP 9100), no existe driver en macOS ni hace falta.

## Anti-metas (qué lo haría fracasar aunque "funcione")

- Que el `.pkg` instale el LaunchAgent para el usuario equivocado (root) y el agente no arranque
  en la sesión gráfica del mostrador.
- Que el asistente abra en un navegador sin contexto gráfico y no se vea.
- Que Gatekeeper bloquee el instalador sin un camino claro (firma/notarización incompletas).
- Que el asistente ofrezca impresoras que no son de tickets (etiquetas Brother) como si sirvieran.

## Restricciones

- Binario Go arm64, macOS 13+, estático (CGO=0), sin dependencias de runtime.
- El agente solo escucha en `127.0.0.1` (no expone puertos a la LAN). No se cambia.
- La IP de la impresora debe ser IPv4 privada (validación existente en `config.Validate`).
- Firma requiere certificados **Developer ID Application** y **Developer ID Installer** más una
  **App Store Connect API key** para `notarytool`. El dueño los provee.

## Enfoques considerados

| Enfoque | Qué aporta | Qué sacrifica |
|---|---|---|
| `.pkg` (elegido) | UX de instalador nativo, postinstall que deja el agente corriendo y abre el asistente | Complejidad de postinstall por usuario |
| `.dmg` (descartado) | Más simple de armar | Requiere arrastrar + un paso manual; no es "instalar y listo" |
| `.zip` + script (descartado) | Lo más liviano | Exige terminal; contradice el objetivo de bajar fricción |

Y para el asistente:

| Enfoque | Qué aporta | Qué sacrifica |
|---|---|---|
| Página web en el agente (elegida) | Reutiliza `/v1/printers` y `/v1/print`; un solo binario; sin firma extra | Es navegador, no ventana nativa |
| App SwiftUI (descartada) | Ventana nativa | Binario nuevo, bundle id, firma y notarización propias, más mantenimiento |

## Diseño de la solución

### 1. Asistente de configuración (dentro del agente, Go)

Nuevos endpoints en `internal/agent/server.go`:

- `GET /` y `GET /setup` → sirven una página HTML autocontenida (CSS + JS inline, sin assets
  externos, texto en español, tono del producto).
- `POST /v1/printer` → guarda `{ "host", "port", "paperWidthMm" }` reusando `config.Validate`
  y `config.Save`. Solo toca esos tres campos (no expone token/secret). Loopback-only como el resto.

Flujo del asistente (JS):

1. Al cargar: `GET /v1/health` para mostrar versión y estado; luego `GET /v1/printers`
   (autodetección existente, `discover.Scan`).
2. Si hay impresoras tipo "ticket": las lista con un botón "Usar esta".
   Si no hay ninguna: muestra el formulario manual (IP, puerto 9100, papel 80/58).
3. Botón "Guardar y probar": `POST /v1/printer` y luego `POST /v1/print` con `{"kind":"test"}`
   (el endpoint ya acepta override de host/port). Muestra resultado en pantalla.
4. Estados de carga/error/vacío explícitos (regla de UI del producto).

### 2. Instalador `.pkg`

- `pkgbuild` empaqueta el binario en `/usr/local/bin/isphub-print-agent`.
- Postinstall (corre como root) que:
  - Instala el LaunchAgent **para el usuario de la consola gráfica** (`stat -f %Su /dev/console`),
    ejecutando `isphub-print-agent install` bajo ese usuario (el comando ya copia el binario a
    `~/Library/Application Support/ISPHubPrintAgent/`, escribe el plist y hace bootstrap/kickstart).
  - Abre `http://127.0.0.1:8787/setup` en el navegador de esa sesión (`launchctl asuser … open`).
- `productbuild` con `Distribution.xml` que incluye Welcome / Read Me / Conclusion (HTML breve)
  y declara macOS 13+ como mínimo.
- Identificadores: paquete `app.isphub.print-agent`, versión `0.2.0`.

### 3. Firma y notarización

- `codesign --options runtime --timestamp` del binario con **Developer ID Application**.
- `pkgbuild … --sign "Developer ID Installer: …"` y `productbuild … --sign "Developer ID Installer: …"`.
- `xcrun notarytool submit … --wait` y `xcrun stapler staple` del `.pkg`.

### 4. Release

- Tag nuevo `v0.2.0` (el `v0.1.0` queda como estaba, con el binario crudo).
- Asset del release: `ISPHub-Print-Agent-0.2.0.pkg` (principal), más `sha256sums.txt`.
- Workflow CI `release.yml` (dispara en tag `v*`) que compila, empaqueta, firma y notariza usando
  secrets de GitHub, y sube el artefacto al release. Además scripts locales (`make pkg`) para
  firmar/notarizar desde esta Mac con los certificados del keychain.

## Qué queda a cargo del implementador

- Texto exacto y estilos de la página del asistente.
- Mecánica exacta del postinstall por usuario (se verifica contra una Mac real).
- Estructura de `Distribution.xml` y textos de Welcome/Read Me/Conclusion.
- Definición exacta de los secrets de CI y del comando de notarización.

## Pendiente del dueño del producto (bloquea la firma, no el código)

- Certificados **Developer ID Application** e **Developer ID Installer** (se generan CSRs aquí
  si se quiere; el alta es en `developer.apple.com`).
- **App Store Connect API key** (Team ID, Issuer ID, Key ID, `.p8`) o, alternativamente, Apple ID
  + contraseña de app para `notarytool`.
