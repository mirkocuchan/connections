package main

import(
	"net/http"
	"github.com/mirkocuchan/connections/internal/auth"
	"encoding/json"
	"github.com/google/uuid"
	"context"
	"github.com/lib/pq"
)
type contextKey string

const userIDKey contextKey = "user_id"

//el middleware toma un handler, y lo devuelve, habiendo ejecutado la funcion interna del middleware antes de ejecutar el handler. 
//el middleware es un wrapper que envuelve al handler, y le agrega funcionalidad antes o despues de ejecutar el handler.
func (s *state) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken, err := auth.GetBearerToken(r.Header)
		if err != nil{
			RespondWithError(w, 401, "Unauthorized")
			return
		}
		userID, err := auth.ValidateJWT(accessToken, s.cfg.JWTSecret)
		if err != nil{
			RespondWithError(w, 401, "Unauthorized")
			return
		}
		//si tuviera next.ServeHTTP(w, r) en la siguiente linea y en vez de tomar el userID usara _, el middleware sería válido. sería un 
		//middleware que solo valida el jwt. si es valido el jwt, llamo al proximo handler. 
		
		//hago que el userID esté disponible para el handler que se ejecuta después del middleware, agregándolo al contexto de la request.
		//entonces uso el middleware para ahorrar el trabajo de parsear el jwt en cada handler que lo necesite. el middleware lo hace una sola vez y lo pone en el contexto de la request para que los handlers posteriores puedan acceder a él. el middleware es un wrapper que envuelve al handler, y le agrega funcionalidad antes o despues de ejecutar el handler.
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
//http.Handler es una interfaz que tiene un solo método: ServeHTTP(ResponseWriter, *Request).
//entonces, http.Handler responde a una request mediante ServeHTTP y cuando hacemos next.ServeHTTP(w, r) entonces, 
//estamos diciendo "ejecutá este handler next con esta request r y este ResponseWriter w."
//resumen de next.ServeHTTP(w, r): el middleware recibe un handler, y lo ejecuta con la request y el response writer que le pasaron al middleware.