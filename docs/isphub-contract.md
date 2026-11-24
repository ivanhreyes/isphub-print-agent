# Contrato entre ISPHub y el agente

El agente ya imprime. ISPHub, en el navegador de la misma Mac, puede usarlo hoy. La cola en el servidor queda descrita aquí para implementarla después, con el mismo estilo que el agente DNS: la Mac sale hacia ISPHub, ISPHub no entra a la LAN.

## Hoy: el navegador de la Mac del mostrador

`POST http://127.0.0.1:8787/v1/print`

`Content-Type: application/json`

Cuerpo: el objeto `data` de `GET /billing/payments/:id/print-receipt`, sin cambios.

Respuesta 200: `{ "ok": true, "bytes": 1234 }`.

Después de un 200, la página pregunta si el papel salió y, si el cajero confirma, llama `POST /billing/payments/:id/print-receipt/confirm` con `{ "confirmed": true }`.

El navegador manda preflight. El agente responde `Access-Control-Allow-Private-Network: true` y refleja el origen solo si es `https://isphub.app`, un subdominio `https://*.isphub.app`, localhost, o un origen listado en `allowedOrigins`.

## Después: cola saliente

Cuando exista, el agente con `serverURL` y `enrollmentToken` hará esto solo.

### Alta

`POST {serverURL}/api/v1/print-agents/enroll`

```json
{ "token": "de un solo uso", "hostname": "mostrador", "version": "0.1.0", "platform": "darwin/arm64" }
```

Respuesta:

```json
{ "data": { "agentId": "…", "agentSecret": "…", "pollIntervalSeconds": 5 } }
```

El token se puede usar una vez. El secreto queda en `config.json` con modo 0600.

### Trabajos

`GET {serverURL}/api/v1/print-agents/jobs`

Cabeceras: `X-Agent-Id`, `X-Agent-Secret`.

```json
{
  "data": {
    "pollIntervalSeconds": 5,
    "jobs": [
      { "id": "job_1", "kind": "payment-receipt", "receipt": { } }
    ]
  }
}
```

`receipt` es el mismo objeto `data` del recibo. `kind` también puede ser `blocks` con `document.format = "ISPHUB_PRINT_V1"`.

### Resultado

`POST {serverURL}/api/v1/print-agents/jobs/{id}/result`

```json
{ "status": "printed" }
```

o `{ "status": "failed", "error": "texto corto" }`.

`serverURL` tiene que ser `https`, salvo `http://127.0.0.1` para desarrollo. Un 404 en estas rutas no detiene el agente: sigue atendiendo el puerto local y reintenta la cola cada 30 segundos.

## Bloques ISPHUB_PRINT_V1

| `type` | Campos |
|---|---|
| `text` | `text`, `align` (`left`, `center`, `right`), `bold`, `size` (`normal`, `large`) |
| `pair` | `left`, `right` |
| `separator` | `char` opcional: `-` o `=` |
| `qr` | `value`, hasta 512 caracteres |
| `feed` | `lines`, de 0 a 8 |

El agente elige PC858 (`ESC t 19`), 32 columnas en 58 mm y 48 en 80 mm, e imprime un corte parcial al final.
