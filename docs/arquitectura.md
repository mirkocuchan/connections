# Arquitectura

Backend monolítico en Go, desplegado en Docker. No usamos microservicios.

## Esquema

App (Expo) → HTTPS/WSS → Caddy → Backend Go
                                  ├─ PostgreSQL (administrado, con copias)
                                  ├─ Almacenamiento de archivos (fotos e historias)
                                  ├─ Resend (mail de recuperación)
                                  ├─ Expo Push (notificaciones)
                                  └─ Sentry (errores)

## Pendientes para producción

- [ ] Configuración por variables de entorno (base de datos, secreto JWT, URL pública, mail)
- [ ] Dockerfile y migraciones de goose al desplegar
- [ ] Endpoint /healthz (servidor + base de datos)
- [ ] Apagado ordenado (cerrar WebSockets y peticiones en curso)
- [ ] Leer la IP real desde X-Forwarded-For (limitador de intentos detrás de Caddy)
- [ ] Logs estructurados (JSON)
- [ ] Archivos en disco persistente; luego Cloudflare R2 o S3
- [ ] Reemplazar logMailer por Resend (dominio con SPF y DKIM)
- [ ] Sentry

## Cuándo separar piezas

- Más de una instancia: mover el hub de WebSocket y el limitador de intentos a Redis.
- Tareas pesadas (procesar imágenes, notificaciones masivas): cola de trabajos.
