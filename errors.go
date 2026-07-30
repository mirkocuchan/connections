package main

import(
	"net/http"
	"encoding/json"
	"log"
)
//cuando se convierta en json, va a quedar como {error: "message de error"}
type ErrorResponse struct {
    Message string `json:"error"`
}
//w implementa una interfaz de Go llamada io.Writer. un io.Writer es cualquier cosa en la que se puedan escribir bytes
//un ResponseWriter tiene un Header, un código y un cuerpo de texto. el content-type nos dice de que consiste lo que nos viene
//el codigo te dice como salio: 200 OK, 500 Internal Server Error y asi
func RespondWithError(w http.ResponseWriter, code int, message string) {
	errorMessage := ErrorResponse{
		Message: message,
	}
	//lo convierte a []byte para poder mandarselo a w.Write
	jsonErrorText, err := json.Marshal(errorMessage)
    if err != nil {
		log.Printf("marshal error")
		w.WriteHeader(http.StatusInternalServerError)
        return
    }

	w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)

    w.Write(jsonErrorText)
}

//func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
//    w.Header().Set("Content-Type", "application/json")
//    w.WriteHeader(code)
//    //json.NewEncoder(w) dice: "todo lo que proceses, no lo guardes en memoria, mandalo directamente al socket de la respuesta HTTP, al nuevo Encoder
//    //.Encode mira qué tiene adentro payload usando reflection, transforma el dato a formato JSON, y hace el stream directo: a medida que genera los bytes en 
//	  //formato JSON, los escribe en w

//    if err := json.NewEncoder(w).Encode(payload); err != nil {
//        log.Printf("Error respondiendo JSON: %v", err)
//    }
//}