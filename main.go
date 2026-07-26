package main

import _ "github.com/lib/pq"
import(
	"database/sql"
	"log"
)
//la estructura va a guardar el tipo "*database.Queries" que es mi dbQueries
type state struct {
    db  *database.Queries
}

func main(){
	//db es del tipo *sql.DB, implementa a la interfaz DBTX. creamos la conexion y nos queda en db
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("error connecting to db: %v", err)
	}
	defer db.Close()

	//luego, creamos un objeto queries que es una estructura que guarda la db connection
	dbQueries := database.New(db)


}
