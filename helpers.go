package main

import(
	"time"
    "net/http"
    "github.com/google/uuid"
    "errors"
    "strings"
    "database/sql"
    "net"
	"encoding/json"
    "os"
    "path/filepath"
    "log"
    "fmt"
    "context"
    "github.com/mirkocuchan/connections/internal/database"
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

//funcion para saber si el campo esta escondido o si se puede ver
func revealOrHidden(visible sql.NullBool, value sql.NullString) string {
    if visible.Valid && visible.Bool {
        return value.String
    }
    return "not revealed yet..."
}

//funcion para saber si el campo esta escondido o si se puede ver (versión Date)
func revealOrHiddenDOB(visible sql.NullBool, value time.Time) string {
    if visible.Valid && visible.Bool {
        return value.Format("2006-01-02") //lo convierto en un string
    }
    return "not revealed yet..."
}

//funcion para saber si hay algun bloqueo entre dos usuarios. devuelve true si hay un bloqueo, false si no lo hay
func (s *state) isBlocked(ctx context.Context, userID1, userID2 uuid.UUID) (bool, error){
    existsBlockBetweenUsersParams := database.ExistsBlockBetweenUsersParams{
        BlockerID: userID1,
        BlockedID: userID2,
    }
    _, err := s.db.ExistsBlockBetweenUsers(ctx, existsBlockBetweenUsersParams)
    if err == sql.ErrNoRows {
        return false, nil // no hay bloqueo y no es un error
    }
    if err != nil {
        return false, err
    }

    return true, nil
}

// true si blockerID bloqueó a blockedID (un solo sentido)
func (s *state) hasBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
    _, err := s.db.ExistsBlockByBlocker(ctx, database.ExistsBlockByBlockerParams{
        BlockerID: blockerID,
        BlockedID: blockedID,
    })
    if err == sql.ErrNoRows {
        return false, nil
    }
    if err != nil {
        return false, err
    }
    return true, nil
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.Split(forwarded, ",")[0]
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func lookupCountryByIP(ip string) (string, error) {
    client := http.Client{Timeout: 2 * time.Second}
    resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=status,countryCode")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
        CountryCode string `json:"countryCode"`
        Status      string `json:"status"`
    }
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Status != "success" {
		return "", errors.New("couldn't determine country from IP")
	}
	return result.CountryCode, nil
}

func normalizeRegionCode(code string) string {
    code = strings.ToUpper(strings.TrimSpace(code))
    if len(code) != 2 {
        return ""
    }
    for _, c := range code {
        if c < 'A' || c > 'Z' {
            return ""
        }
    }
    return code
}

func absoluteURL(r *http.Request, path string) string {
    if !strings.HasPrefix(path, "/") {
        return path
    }
    scheme := "http"
    if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
        scheme = "https"
    }
    return scheme + "://" + r.Host + path
}

// busca el chat entre dos usuarios; si no existe, lo crea
func (s *state) getOrCreateChat(ctx context.Context, userID, otherID uuid.UUID) (database.Chat, error) {
	chat, err := s.db.GetChatByUserIDs(ctx, database.GetChatByUserIDsParams{
		UserOneID: userID,
		UserTwoID: otherID,
	})
	if err == sql.ErrNoRows {
		return s.db.CreateChat(ctx, database.CreateChatParams{
			UserOneID: userID,
			UserTwoID: otherID,
		})
	}
	return chat, err
}

// false si el dueño es anónimo para mí (me inició un chat) o hay bloqueo entre nosotros
func (s *state) canSeeUser(ctx context.Context, viewerID, ownerID uuid.UUID) (bool, error) {
	if viewerID == ownerID {
		return true, nil
	}
	blocked, err := s.isBlocked(ctx, viewerID, ownerID)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, nil
	}
	chat, err := s.db.GetChatByUserIDs(ctx, database.GetChatByUserIDsParams{
		UserOneID: viewerID,
		UserTwoID: ownerID,
	})
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return chat.UserOneID != ownerID, nil
}

// borra del disco un archivo subido, solo si pertenece a ese usuario
func removeUploadedFile(ownerID uuid.UUID, url string) {
	if !strings.HasPrefix(url, "/uploads/") {
		return
	}
	name := filepath.Base(url)
	if !strings.HasPrefix(name, ownerID.String()+"-") {
		return
	}
	if err := os.Remove(filepath.Join("uploads", name)); err != nil && !os.IsNotExist(err) {
		log.Printf("couldn't remove %s: %v", name, err)
	}
}

func (s *state) cleanupExpiredStories() {
	rows, err := s.db.DeleteExpiredStories(context.Background())
	if err != nil {
		log.Printf("cleanup: couldn't delete expired stories: %v", err)
		return
	}
	for _, row := range rows {
		removeUploadedFile(row.UserID, row.MediaUrl)
	}
	if len(rows) > 0 {
		log.Printf("cleanup: %d expired stories removed", len(rows))
	}
}

func (s *state) startCleanupLoop() {
	go func() {
		s.cleanupExpiredStories() // una vez al arrancar
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanupExpiredStories()
		}
	}()
}

// escapa los comodines de LIKE para que se busquen como texto literal
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
