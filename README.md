# Connections

Red social de conexiones anónimas. La idea central: podés hablar con usuarios random sin que sepan quién sos, hasta que decidís (vos, o quien te contactó) revelar tu identidad — de a poco, campo por campo, como un rompecabezas, o de una sola vez.

## Concepto

Cuando alguien te inicia un chat, aparece como un anónimo (`random_xxxxxxxx`). Vos, como quien recibió el mensaje, podés:
- Ponerle un apodo propio (solo vos lo ves).
- Escribir notas libres sobre lo que vas descubriendo charlando.
- Ver los campos que la otra persona decide revelar activamente (ciudad, bio, fecha de nacimiento, fotos, hobbies, idiomas — uno por uno, o todos de una).

La otra persona controla **qué** revela y **cuándo**, campo por campo. No hay reveal automático: cada dato que se muestra fue una decisión explícita de su dueño.

## Stack

- **Lenguaje**: Go
- **Base de datos**: PostgreSQL
- **Migraciones**: [goose](https://github.com/pressly/goose)
- **Queries type-safe**: [sqlc](https://sqlc.dev/)
- **Auth**: JWT (access token corto) + refresh token (hasheado en base, con rotación)
- **Hashing de contraseñas**: argon2id
- **Router**: `net/http` estándar (Go 1.22+, con path params nativos)

## Estructura del proyecto

```
connections/
├── main.go                  # entry point, arma state, corre el servidor
├── handlers.go               # registro de todas las rutas
├── register.go, login.go...  # handlers por dominio
├── types.go                  # tipo Date (fecha sin hora) + Marshal/Unmarshal custom
├── errors.go                  # RespondWithError, RespondWithJSON
├── internal/
│   ├── config/                # carga de variables de entorno
│   ├── database/               # código generado por sqlc (no editar a mano)
│   └── auth/                    # hashing de passwords, JWT, refresh tokens
├── sql/
│   ├── schema/                  # migraciones de goose
│   └── queries/                  # queries fuente para sqlc
└── sqlc.yaml
```

## Variables de entorno (`.env`)

| Variable | Ejemplo | Descripción |
|---|---|---|
| `DB_URL` | `postgres://postgres:pass@localhost:5432/connections` | Conexión a Postgres |
| `PORT` | `:8080` | Puerto del servidor (con los dos puntos) |
| `JWT_SECRET` | (generado con `openssl rand -base64 32`) | Clave para firmar JWTs — nunca subir al repo |
| `ACCESS_TOKEN_DURATION` | `15m` | Vida del access token |
| `REFRESH_TOKEN_DURATION` | `720h` | Vida del refresh token (30 días) |

## Cómo correrlo

```bash
# 1. Instalar goose y sqlc si no los tenés
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# 2. Correr las migraciones
cd sql/schema
goose postgres "postgres://postgres:pass@localhost:5432/connections" up

# 3. Generar el código de queries (solo si tocaste algo en sql/queries/)
cd ../..
sqlc generate

# 4. Levantar el servidor
go run .
```

## Modelo de datos

| Tabla | Qué guarda |
|---|---|
| `users` | Cuenta, credenciales, perfil (bio, ciudad, hobbies, etc.) |
| `refresh_tokens` | Hash del refresh token, expiración, revocación |
| `chats` | Relación 1 a 1 entre dos usuarios (par único, sin importar orden) |
| `messages` | Mensajes de un chat |
| `cards` | La "ficha" — una por cada dirección de cada chat (A sobre B, B sobre A) |
| `user_photos` | Fotos de perfil, ordenadas por posición |
| `stories` | Historias efímeras (24hs), imagen o video |
| `story_views` | Quién vio qué historia (una fila por par historia+usuario) |
| `blocks` | Bloqueos entre usuarios (direccional) |
| `reports` | Reportes con categoría y detalle opcional |

### Sobre `cards` (la Ficha)

Es la tabla más particular del proyecto. Por cada chat, existen **dos** fichas posibles: la que B arma sobre A, y la que A arma sobre B — son entidades independientes, identificadas por `(chat_id, creator_id, subject_id)`, con un índice único sobre esa combinación.

- `creator_id` = quien mira la ficha (le pone apodo, escribe notas).
- `subject_id` = de quién son los datos que se revelan.
- Las columnas `*_visible` son banderas que solo puede tocar el **subject** (revelando algo sobre sí mismo), nunca el creator.

## Endpoints

### Auth
| Método | Ruta | Descripción |
|---|---|---|
| POST | `/register` | Crear cuenta |
| POST | `/login` | Login, devuelve access + refresh token |
| POST | `/refresh` | Renueva el access token (rota el refresh token) |
| POST | `/logout` | Revoca el refresh token |

### Perfil
| Método | Ruta | Descripción |
|---|---|---|
| GET | `/me` | Perfil propio |
| PATCH | `/me` | Editar perfil (parcial, campos opcionales) |

### Chats y mensajes
| Método | Ruta | Descripción |
|---|---|---|
| POST | `/chats` | Iniciar o recuperar un chat con otro usuario |
| GET | `/chats` | Listar mis chats (con nickname resuelto si existe) |
| DELETE | `/chats/{chatID}` | Borrar un chat (cascada a mensajes y fichas) |
| POST | `/chats/{chatID}/messages` | Mandar un mensaje |
| GET | `/chats/{chatID}/messages` | Listar mensajes del chat (polling) |

### Ficha
| Método | Ruta | Descripción |
|---|---|---|
| GET | `/chats/{chatID}/card` | Ver (o crear) la ficha que armé sobre el otro |
| PATCH | `/chats/{chatID}/card/nickname` | Ponerle apodo al otro |
| PATCH | `/chats/{chatID}/card/notes` | Escribir notas sobre el otro |
| PATCH | `/chats/{chatID}/card/reset` | Resetear la ficha (apodo, notas, todo oculto de nuevo) |
| POST | `/chats/{chatID}/card/reveal/{field}` | Revelar un campo propio (`city`, `bio`, `date_of_birth`, `name`, `hobbies`, `languages`, `photos`, `country`) |
| POST | `/chats/{chatID}/card/reveal-all` | Revelar todos los campos propios de una |

### Fotos
| Método | Ruta | Descripción |
|---|---|---|
| POST | `/me/photos` | Agregar una foto |
| GET | `/me/photos` | Listar mis fotos |
| DELETE | `/me/photos/{photoID}` | Borrar una foto propia |

### Discovery
| Método | Ruta | Descripción |
|---|---|---|
| GET | `/users/discover` | Usuarios random para descubrir (excluye chats existentes, bloqueados, y quien no tenga foto) |

### Historias
| Método | Ruta | Descripción |
|---|---|---|
| POST | `/stories` | Crear una historia (vence a las 24hs) |
| GET | `/stories` | Historias activas de todos, con si ya las vi |
| POST | `/stories/{storyID}/view` | Marcar como vista |
| DELETE | `/stories/{storyID}` | Borrar una historia propia |

### Bloqueo y reportes
| Método | Ruta | Descripción |
|---|---|---|
| POST | `/me/block/{userID}` | Bloquear a alguien |
| DELETE | `/me/unblock/{userID}` | Desbloquear |
| GET | `/me/blocked` | Lista de bloqueados |
| POST | `/me/report/{userID}` | Reportar (`reason`: `spam`, `acoso`, `contenido_inapropiado`, `otro`; `details` obligatorio si `otro`) |

El bloqueo se chequea (en ambas direcciones) antes de: crear un chat, mandar un mensaje, ver una ficha, y en el listado de discovery.

## Decisiones de diseño a tener presentes

- **IDs y timestamps los genera Postgres**, no Go (`DEFAULT gen_random_uuid()` / `DEFAULT now()`) — las queries de `INSERT` no los piden como parámetro, se consiguen del `RETURNING *`.
- **Los refresh tokens se guardan hasheados** (SHA-256), nunca en texto plano — mismo criterio que las contraseñas (con argon2id).
- **Los mensajes de error de credenciales son genéricos a propósito** (`"invalid email or password"`), para no permitir enumeración de usuarios.
- **El JWT no se puede revocar antes de tiempo** — por eso su duración es corta (15 min). Revocar el refresh token no invalida un access token ya emitido.

## Pendiente / ideas para más adelante

- Migrar el polling de mensajes e historias a WebSockets.
- Subida real de archivos para fotos/historias (hoy se manda una URL ya resuelta).
- Editar/borrar mensajes con reglas de ventana de tiempo y visto.
- Notificaciones push.
- Panel de revisión de reportes.
