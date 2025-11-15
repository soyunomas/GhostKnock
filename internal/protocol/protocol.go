// El paquete protocol define la estructura de datos utilizada para la comunicación
// entre el cliente y el servidor de GhostKnock.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt" // <<-- ESTA LÍNEA ESTABA AUSENTE. AHORA ESTÁ AQUÍ.
	"time"
)

// Payload es la estructura de datos que el cliente envía al servidor.
// Contiene la información necesaria para que el servidor verifique la solicitud
// y decida si ejecuta una acción.
type Payload struct {
	Timestamp int64             `json:"timestamp"`
	ActionID  string            `json:"action_id"`
	Params    map[string]string `json:"params,omitempty"`
}

// NewPayload crea una nueva instancia de Payload con la marca de tiempo actual.
func NewPayload(actionID string) *Payload {
	return &Payload{
		Timestamp: time.Now().UnixNano(),
		ActionID:  actionID,
		Params:    make(map[string]string),
	}
}

// Serialize convierte el Payload a un slice de bytes (JSON) para su transmisión.
func (p *Payload) Serialize() ([]byte, error) {
	if p.ActionID == "" {
		return nil, errors.New("ActionID no puede estar vacío")
	}
	return json.Marshal(p)
}

// DeserializePayload convierte un slice de bytes de nuevo a una struct Payload.
func DeserializePayload(data []byte) (*Payload, error) {
	var p Payload
	err := json.Unmarshal(data, &p)
	if err != nil {
		// Envolvemos el error para dar más contexto.
		return nil, fmt.Errorf("fallo al deserializar el payload: %w", err)
	}
	// Validamos que el payload deserializado contenga los campos mínimos requeridos.
	if p.ActionID == "" {
		return nil, errors.New("el payload deserializado no contiene ActionID")
	}
	return &p, nil
}
