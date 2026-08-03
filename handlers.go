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

//net/http exige que un handler tenga la firma (ResponseWriter, *Request). lo hacemos a register un método de state para tener acceso a db y cfg desde adentro sin recibirlos como parámetro,
//porque no permite tener state de parametro al ser un handler

//creamos un tipo igual que time.Time. por que? porque cuando viene un time.Time, unmarshal no sabe que hacer. va a usar unmarshalJSON, pero ve "YYYY-MMM-DDD" y le falta información
//no sabe como unmarshallear, por eso creamos este type. encoding/json pregunta: este tipo tiene unmarshalJSON function? por eso la creamos, para que pueda unmarshalear
type Date time.Time

//Date es el R3ECEIVER porque el método necesita saber sobre qué instancia de Date está actuando.
func (d *Date) UnmarshalJSON(data []byte) error{
	//convertir los bytes JSON a un string, sin comillas. nosotros recibimos con comillas la fecha esa y la convertimos en string
    convertedString := strings.Trim(string(data), `"`)

    //parsear usando el formato YYYY-MM-DD, crea un time.Time con ese estilo para la fecha, pero con la hora y todos los detalles que necesitamos
    modifiedTime, err := time.Parse("2006-01-02", convertedString)
    if err != nil {
        return err
    }

    //guardar el resultado en el receiver
	//estamos escribiendo directamente en el lugar de memoria donde vive user.DateOfBirth
    *d = Date(modifiedTime)
    return nil
}

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

//el state es el receiver, (el objeto que está ejecutando el método)
//cuando handlers() escribe s.register, ese s es el mismo que le llegó a handlers(),  necesita recibir la instancia de alguna manera 
func (s *state) handlers() {
	//handle toma si o si del tipo handler, y como s.register no lo es, la convertimos con handlerfunc
	http.Handle("POST /register", http.HandlerFunc(s.register))
}