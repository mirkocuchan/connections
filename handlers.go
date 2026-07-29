package main

import(
	"net/http"
)
//net/http exige que un handler tenga la firma (ResponseWriter, *Request). lo hacemos a register un método de state para tener acceso a db y cfg desde adentro sin recibirlos como parámetro,
//porque no permite tener state de parametro al ser un handler
func (s *state) register(w http.ResponseWriter, r *http.Request) {
	
}
//el state es el receiver, (el objeto que está ejecutando el método)
//cuando handlers() escribe s.register, ese s es el mismo que le llegó a handlers(),  necesita recibir la instancia de alguna manera 
func (s *state) handlers() {
	//handle toma si o si del tipo handler, y como s.register no lo es, la convertimos con handlerfunc
	http.Handle("POST /register", http.HandlerFunc(s.register))
}