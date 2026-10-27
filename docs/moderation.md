# Moderación de Connections

## Principios
- Ningún reporte produce una sanción automática. Toda sanción la decide una persona, mirando la evidencia.
- Un reporte es una señal, no una prueba. Se resuelve con lo que dice la conversación guardada.
- Revisar la cola al menos una vez por día. Los casos graves (amenazas, contenido sexual con menores, extorsión) se atienden el mismo día.

## Flujo de revisión
1. Abrir la consola de la base:
   `docker compose exec db psql -U postgres -d users`  (activar vista vertical con `\x`)
2. Ver reportes pendientes:
   ```sql
   SELECT report_id, created_at, reason, details, reported_id, reported_username, chat_id
   FROM reports WHERE status = 'pending' ORDER BY created_at;
   ```
3. Leer la evidencia congelada de uno (reemplazar el id real):
   ```sql
   SELECT author_role, content, sent_at FROM report_messages
   WHERE report_id = '<report_id>' ORDER BY sent_at;
   ```
4. Decidir:
   - **Banear** (también pasa sus reportes pendientes a `actioned`, revoca sus sesiones y lo expulsa del chat):
     ```bash
     curl -i -X POST http://<host>/admin/users/<user_id>/ban \
       -H "X-Admin-Key: $ADMIN_KEY" -H "Content-Type: application/json" \
       -d '{"reason":"<motivo concreto>"}'
     ```
   - **Descartar**:
     ```sql
     UPDATE reports SET status = 'dismissed', reviewed_at = now() WHERE report_id = '<report_id>';
     ```
5. **Desbanear** si fue un error:
   `curl -i -X POST http://<host>/admin/users/<user_id>/unban -H "X-Admin-Key: $ADMIN_KEY"`

## Criterios
- Ban: contenido sexual no solicitado, amenazas, acoso persistente, estafas o spam masivo, y declaración explícita de ser menor de 18 años dentro de la conversación.
- Descartar: desacuerdos personales, reportes sin evidencia, o conducta que no rompe las reglas de la comunidad.
- Contenido sexual que involucre menores: banear de inmediato, conservar la evidencia y denunciar ante las autoridades competentes.

## Reportes abusivos
Reportar a alguien por venganza o para molestar rompe las reglas de la comunidad. Para detectar a quien lo hace (cuentas con 3 o más reportes descartados):
```sql
SELECT reporter_id,
       count(*) FILTER (WHERE status = 'dismissed') AS descartados,
       count(*) AS total
FROM reports WHERE reporter_id IS NOT NULL
GROUP BY reporter_id
HAVING count(*) FILTER (WHERE status = 'dismissed') >= 3
ORDER BY descartados DESC;
```
Ya existe un límite por usuario en la cantidad de reportes y el rechazo de duplicados recientes.

## Notas
- Un baneado no puede iniciar sesión, usar la API ni conectarse al chat.
- Como moderador se ve la identidad real detrás de los alias `anon-xxxx`. Esto debe constar en la política de privacidad.
- La clave de admin (`ADMIN_KEY`) vive solo en variables de entorno; nunca en el repositorio.

## Pendientes
- Definir cuánto tiempo se conserva la evidencia de los reportes (`report_messages`).
- Evaluar un proceso de apelación si hay volumen.
- En producción, bloquear `/admin` desde afuera (proxy) y acceder por túnel.
