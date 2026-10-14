package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/mirkocuchan/connections/internal/auth"
)

//client = una conexión abierta (un celular, una pestaña).
//send es su "buzón": otros handlers dejan mensajes ahí y la goroutine de la conexión los escribe al socket.
type client struct {
	send chan []byte
}

//hub = quién está conectado. userID -> conjunto de conexiones (un usuario puede tener más de una a la vez). ej: userID de Mirko  →  { conexión del iPhone, conexión de otra pestaña }
type hub struct {
	mu      sync.RWMutex //protege el canal send de accesos concurrentes
	clients map[uuid.UUID]map[*client]struct{}
}

func newHub() *hub {
	return &hub{clients: make(map[uuid.UUID]map[*client]struct{})}
}

func (h *hub) add(userID uuid.UUID, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*client]struct{})
	} //creo mi conjunto de conexiones si no existía
	h.clients[userID][c] = struct{}{}
}

func (h *hub) remove(userID uuid.UUID, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[userID], c)
	if len(h.clients[userID]) == 0 {
		delete(h.clients, userID)
	}
}

//sendToUser manda un evento a todas las conexiones del usuario. si no está conectado no hace nada: el push y el REST cubren ese caso.
func (h *hub) sendToUser(userID uuid.UUID, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock() //mandar al mapa no cuenta como "escribir" en él, pero sí hay que proteger el canal send de accesos concurrentes. muchos leen en paralelo, pero si alguien escribe (cierra el canal) hay que bloquear.
	defer h.mu.RUnlock()
	for c := range h.clients[userID] {
		select {
		case c.send <- data:
		default:
			//buzón lleno (cliente lento): descartamos. Es un "hint", el cliente se resincroniza con REST al reconectar.
		}
	}
}

//disconnectUser corta todas las conexiones de un usuario (para logout).
func (h *hub) disconnectUser(userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients[userID] {
		close(c.send) //la goroutine ve el canal cerrado y termina
	}
	delete(h.clients, userID) //cerramos todos los canales y borramos el mapa de conexiones del usuario. si se reconecta, se crea uno nuevo.
	//si no borramos el mapa, queda un mapa vacío y la próxima vez que se conecte el usuario, el bucle de sendToUser no hace nada (porque el mapa está vacío) pero sigue existiendo y bloquea a los demás.
}

func (s *state) wsHandler(w http.ResponseWriter, r *http.Request) {
	//react native no puede mandar header Authorization en un WebSocket, entonces token por query.
	//token a la vista en los logs, aceptable me dijo claude...
	token := r.URL.Query().Get("token")
	userID, err := auth.ValidateJWT(token, s.cfg.JWTSecret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	//el servidor ya está esperando pedidos HTTP, la app manda pedido, servidor contesta y se corta
	//queremos dejar abierto para que el servidor hable. para “llamada abierta” la app manda un pedido HTTP especial que dice “quiero mejorar esta conexión a WebSocket” 
	//accept es la parte del servidor que: mira ese pedido y comprueba que sea válido, dice “dale, desde ahora usamos WebSocket” y te entrega conn, la línea abierta.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return //accept ya respondió el error
	}
	defer conn.CloseNow()

	c := &client{send: make(chan []byte, 16)}
	s.hub.add(userID, c)
	defer s.hub.remove(userID, c)
	log.Printf("ws: conectado")

	//closeRead lee en segundo plano (necesario para procesar pongs y cierres) y devuelve un ctx que se cancela cuando el cliente se va (multiples razones, ya segundo plano). si no hacemos esto, el select de abajo nunca termina y la goroutine queda colgada.
	ctx := conn.CloseRead(r.Context())

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done(): //si se cerro la conexión, se cancela el ctx y termina la goroutine. 
			return
		//case mensaje para mandar al cliente: lo leemos del buzon (canal send) y lo escribimos al socket.
		case msg, ok := <-c.send: //leemos desde el buzon, mismo canal donde sendToUser manda los mensajes. si el canal se cerró (disconnectUser) ok = false y terminamos la goroutine.
			if !ok { //disconnectUser cerró el canal
				conn.Close(websocket.StatusNormalClosure, "bye")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, msg)
			cancel() //cancel() libera los recursos del contexto con timeout, no con defer porque queremos liberar los recursos inmediatamente después de usarlo, no al final de la función, porquye se acumularian los defer
			if err != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pctx) // espera el pong; si no llega, conexión muerta
			cancel()
			if err != nil {
				return
			}
		}
	}
}