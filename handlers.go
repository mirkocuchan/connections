package main

import(
	"net/http"
	"io"
	"github.com/mirkocuchan/connections/internal/auth"
	"github.com/mirkocuchan/connections/internal/database"
	"time"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"github.com/lib/pq"
)

type receivedUser struct {
	Username string
	Email string
	Password string
	DateOfBirth Date
}

func (s *state) register(w http.ResponseWriter, r *http.Request){
	defer r.Body.Close()

	userData, err := io.ReadAll(r.Body)
	if err != nil{
		RespondWithError(w, 400, "couldn't read the request body")
		return
	}
	
	var user receivedUser
	if err := json.Unmarshal(userData, &user); err != nil {
        RespondWithError(w, 400, "error unmarshalling JSON")
		return
    }
	hashedPassword, err := auth.HashPassword(user.Password)
	if err != nil{
		RespondWithError(w, 400, "error hashing the password")
		return
	}
	//asigno un Date a un campo que espera time.Time? NO. necesito hacer una conversión: time.Time(user.DateOfBirth). 
	//válida porque Date tiene la misma estructura interna que time.Time.
	userParams := database.CreateUserParams{
		Username: user.Username,
		Email: user.Email,
		PasswordHash: hashedPassword,
		DateOfBirth: time.Time(user.DateOfBirth),
		}
	
	createdUser, err := s.db.CreateUser(r.Context(), userParams)
	//cuando Postgres rechaza un INSERT por violar el UNIQUE de username o email, no te devuelve un error genérico de Go. 
	//te devuelve un error que tiene información específica de Postgres: un código de error estandarizado. 
	//ese código para "unique violation" es siempre 23505. el paquete lib/pq convierte esa respuesta de PostgreSQL en una estructura de Go (struct) llamada pq.Error.
	//expone esa estructura (es decir, hacer que sus campos sean públicos, con mayúscula), te permite hacer un type assertion (conversión de tipo) en Go para acceder directamente a esos campos

	if err != nil{
		//quiero comprobar si detrás de esta interfaz de error, hay un pq.Error. si lo hay quiero acceso a él con todos sus campos propios.
		//variable, ok := interfaz.(TipoConcreto). ok es bool: true si err era *pq.Error por dentro, false si no. pqErr es la variable nueva, de tipo *pq.Error
		//si ok = true, podemos acceder a pqErr.Code porque ahora Go sabe con certeza qué tipo es. ok = false, pqErr va a ser nil (no te sirve, no rompe nada)
		if pqErr, ok := err.(*pq.Error); ok {
			//pqErr es de tipo concreto pq.Err ya
			if pqErr.Code == "23505"{
				RespondWithError(w, 409, "there is another user with this username")
				return
			}else{
				RespondWithError(w, 500, "error creating a user in the database")
				return
			}
		}else{
			RespondWithError(w, 500, "error creating a user in the database")
			return
		}
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	
	RespondWithJSON(w, http.StatusCreated, responseUser{
		ID: createdUser.UserID,
		Username: createdUser.Username,
		Email: createdUser.Email,
		DateOfBirth: Date(createdUser.DateOfBirth),
		CreatedAt: createdUser.CreatedAt,
		UpdatedAt: createdUser.UpdatedAt,
	})
}

func (s *state) login(w http.ResponseWriter, r *http.Request){
	type User struct{
		Email string    `json:"email"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	params := User{}
	
	err := decoder.Decode(&params)
	if err != nil {
		RespondWithError(w, 400, "couldn't decode the body")
		return
	}
	user, err := s.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil{
		RespondWithError(w, 400, "Incorrect email or password")
		return
	}

	rightPassword, err := auth.CheckPasswordHash(params.Password, user.PasswordHash)
	if err != nil{
		RespondWithError(w, 400, "Incorrect email or password")
		return
	}
	if rightPassword == false{
		RespondWithError(w, 401, "Incorrect email or password")
		return
	}
		
	expiresIn := s.cfg.AccessTokenDuration
	newJWT, err := auth.MakeJWT(user.UserID, s.cfg.JWTSecret, expiresIn)
	if err != nil{
		RespondWithError(w, 400, "couldnt't generate the JWT")
		return
	}
	refreshTokenString := auth.MakeRefreshToken()
	hashedRefreshToken := auth.HashRefreshToken(refreshTokenString)
	expiresAt := time.Now().Add(s.cfg.RefreshTokenDuration)

	newRefreshToken := database.CreateRefreshTokenParams{
		TokenHash: hashedRefreshToken,
		UserID: user.UserID,
		ExpiresAt: expiresAt,
	}

	_, err = s.db.CreateRefreshToken(r.Context(), newRefreshToken)
	if err != nil{
		RespondWithError(w, 401, "couldn't generate a refresh token")
		return
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Token string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	RespondWithJSON(w, 200, responseUser{ID: user.UserID, Username: user.Username, Email: user.Email,
	DateOfBirth: Date(user.DateOfBirth), CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt, Token: newJWT,
	RefreshToken: refreshTokenString})
	return
}

func (s *state) refresh(w http.ResponseWriter, r *http.Request){
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
        return
	}

	hashedRefreshToken := auth.HashRefreshToken(refreshToken)
	refreshTokenData, err := s.db.GetRefreshTokenWithHash(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't find the refresh token in the database")
		return
	}
	if refreshTokenData.ExpiresAt.Before(time.Now()){
		RespondWithError(w, 400, "the refresh token has expired")
		return
	}
	if refreshTokenData.RevokedAt.Valid{
		RespondWithError(w, 400, "the refresh token has been revoked")
		return
	}
	//si existe me fijo si está revocado o expirado. si no, genero un nuevo JWT y un nuevo refresh token, y devuelvo ambos al cliente.
	expiresIn := s.cfg.AccessTokenDuration
	newJWT, err := auth.MakeJWT(refreshTokenData.UserID, s.cfg.JWTSecret, expiresIn)
	if err != nil{
		RespondWithError(w, 400, "couldnt't generate the JWT")
		return
	}

	refreshTokenString := auth.MakeRefreshToken()
	newHashedRefreshToken := auth.HashRefreshToken(refreshTokenString)
	expiresAt := time.Now().Add(s.cfg.RefreshTokenDuration)

	newRefreshToken := database.CreateRefreshTokenParams{
		TokenHash: newHashedRefreshToken,
		UserID: refreshTokenData.UserID,
		ExpiresAt: expiresAt,
	}
	_, err = s.db.CreateRefreshToken(r.Context(), newRefreshToken)
	if err != nil{
		RespondWithError(w, 401, "couldn't generate a refresh token")
		return
	}
	err = s.db.RevokeRefreshToken(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't revoke the old refresh token")
		return
	}

	RespondWithJSON(w, 200, map[string]string{"token": newJWT, "refresh_token": refreshTokenString})
}

func (s *state) logout(w http.ResponseWriter, r *http.Request){
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
        return
	}

	hashedRefreshToken := auth.HashRefreshToken(refreshToken)
	err = s.db.RevokeRefreshToken(r.Context(), hashedRefreshToken)
	if err != nil{
		RespondWithError(w, 400, "couldn't revoke the refresh token")
		return
	}
	RespondWithJSON(w, 200, map[string]string{"message": "refresh token revoked successfully"})
}

func (s *state) getMe(w http.ResponseWriter, r *http.Request){
	//quiero obtener el userID del contexto de la request. el middleware authMiddleware lo puso ahí, así que si llegamos a este punto, el userID debería estar en el contexto.
	userID, err := s.getUserIDFromContext(r)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil{
		RespondWithError(w, 401, "Unauthorized")
		return
	}
	type responseUser struct {
		ID uuid.UUID `json:"id"`
		Username string `json:"username"`
		Email string `json:"email"`
		DateOfBirth Date `json:"date_of_birth"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	
	RespondWithJSON(w, 200, responseUser{
		ID: user.UserID,
		Username: user.Username,
		Email: user.Email,
		DateOfBirth: Date(user.DateOfBirth),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	})
}
	
//el state es el receiver, (el objeto que está ejecutando el método)
//cuando handlers() escribe s.register, ese s es el mismo que le llegó a handlers(),  necesita recibir la instancia de alguna manera 
func (s *state) handlers() {
	//handle toma si o si del tipo handler, y como s.register no lo es, la convertimos con handlerfunc
	http.Handle("POST /register", http.HandlerFunc(s.register))
	http.Handle("POST /login", http.HandlerFunc(s.login))
	http.Handle("POST /refresh", http.HandlerFunc(s.refresh))
	http.Handle("POST /logout", http.HandlerFunc(s.logout))

	http.Handle("GET /me", s.authMiddleware(http.HandlerFunc(s.getMe)))
	//s.getMe es un handler, pero no cumple la interfaz http.Handler. entonces lo envolvemos en http.HandlerFunc para que cumpla la interfaz.
	//s.authMiddleware es un middleware que toma un handler y devuelve un handler. entonces le pasamos el handler envuelto en http.HandlerFunc, y nos devuelve otro handler que valida el JWT antes de ejecutar s.getMe.
	//entonces, cuando llega una request a /me, primero pasa por s.authMiddleware, que valida el JWT y pone el userID en el contexto de la request, y luego ejecuta s.getMe con ese contexto.
	//s.getMe puede acceder al userID del contexto de la request gracias a s.authMiddleware. si no hubiera pasado por el middleware, s.getMe no podría acceder al userID y devolvería un error 401.
	//resumen: s.getMe (tu handler) → envuelto en http.HandlerFunc (para que cumpla la interfaz http.Handler que authMiddleware espera recibir) → envuelto en s.authMiddleware (que valida el JWT antes de dejarlo pasar) → registrado con http.Handle.
}
