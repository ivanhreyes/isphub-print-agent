# Agente de impresión ISPHub para Mac

Imprime recibos de ISPHub en una mini printer **EC Line EC-PM-80330** (papel de 80 mm, ESC/POS, puerto 9100) desde una Mac con chip Apple Silicon. Un solo binario. No hace falta Node, Python ni el cuadro de impresión del navegador.

Sirve en Mac M1, M2, M3 y M4 con macOS 13 o posterior. El binario es arm64 y el runtime de Go pide esa versión mínima; cualquier Mac con chip M1 puede instalar macOS 13. No sirve en Mac Intel.

## Quick path

1. Descarga `isphub-print-agent` para arm64, o compílalo con `make build`.
2. Busca la impresora y guarda su IP:

```bash
./isphub-print-agent discover
./isphub-print-agent config --host 192.168.0.20 --paper 80
./isphub-print-agent print-test
```

3. Instálalo para el usuario actual. Arranca al iniciar sesión y escucha en `127.0.0.1:8787`:

```bash
./isphub-print-agent install
```

4. En la Mac del mostrador, ISPHub envía el JSON del recibo a `POST http://127.0.0.1:8787/v1/print`.

La primera vez que macOS baje el binario de internet, si no abre: clic derecho → Abrir, o `xattr -d com.apple.quarantine isphub-print-agent`. El proyecto no está firmado con un certificado de Apple.

## Qué imprime

| Trabajo | Cómo llega |
|---|---|
| Recibo de pago | El mismo JSON de `GET /billing/payments/:id/print-receipt`, el objeto `data` o `{ "data": ... }` |
| Página de prueba | `{"kind":"test"}` o el comando `print-test` |
| Ticket genérico | `{"kind":"blocks","document":{"format":"ISPHUB_PRINT_V1",...}}` |

El recibo respeta el ancho de 58 o 80 mm, las casillas del ajuste «Recibo térmico», el aviso de reimpresión y el QR de `https://isphub.app/app`. El logo de la organización no se imprime en esta versión: sí salen el nombre, el RFC y el resto del texto.

La impresora de esta clase se reconoce sola: página web con título `Configures`, charset `gb2312` y puerto 9100. Una Brother QL que también use el 9100 se ignora y no recibe comandos ESC/POS.

## API local

Solo acepta conexiones en `127.0.0.1`. Un sitio que no sea ISPHub (`https://isphub.app` o un subdominio) recibe 403.

| Método | Ruta | Uso |
|---|---|---|
| `GET` | `/v1/health` | Comprueba que el agente está vivo |
| `GET` | `/v1/printers` | Busca impresoras en la LAN. No imprime |
| `POST` | `/v1/print` | Imprime. Cuerpo JSON |

Ejemplo:

```bash
curl -s -H 'Content-Type: application/json' \
  -d '{"kind":"test"}' \
  http://127.0.0.1:8787/v1/print
```

Cuando el navegador confirma que el papel salió, ISPHub sigue usando `POST /billing/payments/:id/print-receipt/confirm`. El agente no marca la reimpresión por su cuenta.

El contrato para que el servidor encole trabajos y esta Mac los recoja está en [docs/isphub-contract.md](docs/isphub-contract.md). Hasta que esa cola exista, el camino es el `POST` local.

## Configuración

Archivo: `~/Library/Application Support/ISPHubPrintAgent/config.json` (permisos 0600).

| Campo | Valor |
|---|---|
| `printerHost` | IP privada de la EC-PM-80330 |
| `printerPort` | `9100` |
| `paperWidthMm` | `80` en esta impresora. El recibo puede pedir 58 |
| `listen` | `127.0.0.1:8787` |
| `serverURL` | Vacío hasta que ISPHub publique la cola. Tiene que ser `https` |
| `dryRun` | `true` guarda el trabajo en la respuesta y no abre la impresora |

Logs: `~/Library/Logs/ISPHubPrintAgent/`.

```bash
isphub-print-agent status
isphub-print-agent uninstall
```

`uninstall` deja el archivo de configuración.

## Compilar

Hace falta Go 1.25 o posterior, en la Mac donde compilas. Quien solo imprime no lo necesita.

```bash
make build
make test
```

El binario queda en `dist/isphub-print-agent`. Es Mach-O arm64 y usa solo la biblioteca del sistema.

## Límites

- No abre puertos hacia la red del negocio. La impresora sí está en la LAN; el agente se conecta a ella.
- Rechaza IPs públicas.
- No imprime fichas hotspot en HTML ni etiquetas Brother.
- No incluye el logo en mapa de bits.

## Licencia

MIT. Ver [LICENSE](LICENSE).
