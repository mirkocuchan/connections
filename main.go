package main

import _ "github.com/lib/pq"
import (
	"context"
	"database/sql"
	"github.com/mirkocuchan/connections/internal/config"
	"github.com/mirkocuchan/connections/internal/database"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// la estructura va a guardar el tipo "*database.Queries" que es mi dbQueries
type state struct {
	db     *database.Queries
	cfg    *config.Config
	hub    *hub
	mailer Mailer
}

func main() {
	//getting the dbURL from the config
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("error reading config: %v", err)
	}
	uploadDir = cfg.UploadDir
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		log.Fatalf("couldn't create upload dir %s: %v", uploadDir, err)
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
		db:     dbQueries,
		cfg:    &cfg,
		hub:    newHub(),
		mailer: logMailer{},
	}
	//inicializo la libreta con los handlers, me va a permitir usar las rutas y las funciones
	programState.handlers()
	programState.startCleanupLoop()

	//healthcheck: 200 si la API y la base responden, 503 si la base no contesta. es llamada
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})

	//usamos http.Server en vez de http.ListenAndServe para poder apagarlo con Shutdown.
	//Handler: nil, si no lo pones, lo deja por defecto como Handler: nil y eso significa que usa el mux por defecto, donde ya registraste tus rutas, la libreta que defini en .handlers()
	srv := &http.Server{
		Addr:              cfg.PORT,         //programa corre dentro del contenedor, y ":8080" significa “escuchá el 8080 en la red del contenedor”. docker, con ports: "8080:8080", reenvía desde el 8080 de mi PC hasta ahí.
		ReadHeaderTimeout: 10 * time.Second, //corta clientes que mandan los headers a cuentagotas, possible attack, slowloris
	}

	//ctx se cancela cuando llega Ctrl+C (os.Interrupt) o SIGTERM (lo que manda Docker/hosting)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	//el servidor corre en una goroutine para que main pueda esperar la señal de apagado
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()

	select {
	case err := <-serverErr:
		//falló antes de que alguien pidiera apagar (por ejemplo, puerto ocupado)
		log.Fatalf("port listening error: %v", err)
	case <-ctx.Done():
		log.Println("shutting down...")
	}
	stop() //durante el apagado, un segundo Ctrl+C mata el proceso en el acto

	//damos hasta 10s para que terminen las requests en curso
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}
