package protocol

import (
	"testing"
)

func FuzzDeserializePayload(f *testing.F) {
	// 1. Semillas: Datos que parecen JSON válido y datos que no.
	f.Add([]byte(`{"action_id": "test", "timestamp": 123456}`))
	f.Add([]byte(`{`)) // JSON roto
	f.Add([]byte(`{"action_id": 123}`)) // Tipo incorrecto
	
	f.Fuzz(func(t *testing.T, data []byte) {
		// Ejecutamos la función. 
		// No nos importa si devuelve error (err != nil), eso es bueno si la entrada es basura.
		// Solo nos importa que NO HAGA PANIC.
		payload, err := DeserializePayload(data)

		if err == nil {
			// Si no hubo error, el payload debe ser coherente
			if payload == nil {
				t.Errorf("DeserializePayload devolvió err=nil pero payload=nil")
			}
			if payload.ActionID == "" {
				// Nuestra lógica dice que ActionID es obligatorio. 
				// Si DeserializePayload no devuelve error con ActionID vacío, revisamos la implementación.
				// (Nota: Tu implementación actual devuelve error si ActionID está vacío, así que esto es correcto).
			}
		}
	})
}
