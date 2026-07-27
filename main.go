package main

import _ "github.com/lib/pq"
import(
	"database/sql"
	"github.com/mirkocuchan/connections/internal/config"
	"github.com/mirkocuchan/connections/internal/database"
	"log"
)
//la estructura va a guardar el tipo "*database.Queries" que es mi dbQueries
type state struct {
    db  *database.Queries
	cfg *config.Config
}

func main(){
	//getting the dbURL from the config
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("error reading config: %v", err)
	}
	
	//db es del tipo *sql.DB, implementa a la interfaz DBTX. creamos la conexion y nos queda en db
	db, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		log.Fatalf("error connecting to db: %v", err)
	}
	defer db.Close()

	//luego, creamos un objeto queries que es una estructura que guarda la db connection
	dbQueries := database.New(db)
	//definimos el estado del programa con la config y la db
	programState := &state{
		db:  dbQueries,
		cfg: &cfg,
	}

}
