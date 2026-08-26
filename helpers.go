package main

import(
	"time"
    "net/http"
    "github.com/google/uuid"
    "errors"
    "strings"
    "database/sql"
    "fmt"
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

func (d Date) MarshalJSON() ([]byte, error){
    
    convertedD := time.Time(d)
    //no hereda los métodos del tipo original, time, entonces tenemos que convertirlo para poder formatearlo y hacerlo string

    dateInAString := convertedD.Format("2006-01-02")
    completedStringOfDate := fmt.Sprintf("\"%s\"", dateInAString) //lo envuelvo en comillas
    
    return []byte(completedStringOfDate), nil //respeto la interfaz de json.Marshal y devuelvo un []byte, lo convierto a byte

}

//función que recibe un request y devuelve el userID del contexto de la request. si no hay userID, devuelve uuid.Nil y un error
func (s *state) getUserIDFromContext(r *http.Request) (uuid.UUID, error) {
	userIDValue := r.Context().Value(userIDKey)
	if userIDValue == nil {
		return uuid.Nil, errors.New("user ID not found in context")
	}
    //type assertion: userIDValue es de tipo interface{}, necesitamos convertirlo a uuid.UUID
    //por que es del tipo inteface{}? porque context.WithValue devuelve un contexto con un valor de tipo interface{}, que puede ser cualquier cosa. cuando lo recuperamos, lo obtenemos como interface{} y necesitamos convertirlo al tipo que sabemos que es.
	userID, ok := userIDValue.(uuid.UUID)
    if !ok {
        return uuid.Nil, errors.New("user ID in context is not of type uuid.UUID")    
    }

	return userID, nil
}

//funcion que convierte un *string en un sql.Nullstring así puede ser tomado
func nullString(s *string) sql.NullString {
    if s == nil {
        return sql.NullString{}
    }

    return sql.NullString{
        String: *s,
        Valid:  true,
    }
}