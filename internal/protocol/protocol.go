// El paquete protocol define la estructura de datos utilizada para la comunicación
// entre el cliente y el servidor de GhostKnock.
package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512" // <<-- NUEVA IMPORTACIÓN
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/nacl/box"
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

// --- INICIO: Lógica de Cifrado y Firma (Protocolo v2) ---

const (
	NonceSize = 24
)

// EncryptAndSign crea un paquete seguro y cifrado.
// Devuelve: [firma (64 bytes)][nonce (24 bytes)][payload cifrado]
func EncryptAndSign(payload *Payload, clientPrivKey ed25519.PrivateKey, serverPubKey ed25519.PublicKey) ([]byte, error) {
	payloadBytes, err := payload.Serialize()
	if err != nil {
		return nil, fmt.Errorf("no se pudo serializar el payload: %w", err)
	}

	// Convertir claves a formato Curve25519 para nacl/box
	boxClientPrivKey := privateKeyToCurve25519(clientPrivKey)
	boxServerPubKey, ok := publicKeyToCurve25519(serverPubKey)
	if !ok {
		return nil, errors.New("la clave pública del servidor no es válida para cifrado")
	}

	var nonce [NonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("no se pudo generar el nonce: %w", err)
	}

	encryptedPayload := box.Seal(nil, payloadBytes, &nonce, &boxServerPubKey, &boxClientPrivKey)

	messageToSign := append(nonce[:], encryptedPayload...)
	signature := ed25519.Sign(clientPrivKey, messageToSign)

	finalMessage := append(signature, messageToSign...)
	return finalMessage, nil
}

// VerifyAndDecrypt valida, descifra y devuelve el payload de un paquete seguro.
func VerifyAndDecrypt(message []byte, clientPubKey ed25519.PublicKey, serverPrivKey ed25519.PrivateKey) (*Payload, error) {
	if len(message) <= ed25519.SignatureSize+NonceSize {
		return nil, errors.New("mensaje demasiado corto para ser válido")
	}

	signature := message[:ed25519.SignatureSize]
	encryptedPart := message[ed25519.SignatureSize:]

	if !ed25519.Verify(clientPubKey, encryptedPart, signature) {
		return nil, errors.New("la firma es inválida")
	}

	var nonce [NonceSize]byte
	copy(nonce[:], encryptedPart[:NonceSize])
	ciphertext := encryptedPart[NonceSize:]

	// Convertir claves a formato Curve25519 para nacl/box
	boxServerPrivKey := privateKeyToCurve25519(serverPrivKey)
	boxClientPubKey, ok := publicKeyToCurve25519(clientPubKey)
	if !ok {
		return nil, errors.New("la clave pública del cliente no es válida para descifrado")
	}

	decryptedPayloadBytes, ok := box.Open(nil, ciphertext, &nonce, &boxClientPubKey, &boxServerPrivKey)
	if !ok {
		return nil, errors.New("fallo al descifrar el payload (clave incorrecta o mensaje corrupto)")
	}

	return DeserializePayload(decryptedPayloadBytes)
}

// --- INICIO: Conversores de Clave Ed25519 -> Curve25519 ---

// <<-- INICIO: FUNCIÓN CORREGIDA -->>
func privateKeyToCurve25519(priv ed25519.PrivateKey) [32]byte {
	// La especificación (RFC 8032) para convertir una clave privada Ed25519 a una
	// clave secreta X25519 requiere hashear la semilla (los primeros 32 bytes de la
	// clave privada) con SHA-512, tomar los primeros 32 bytes del hash y
	// aplicar "clamping".
	h := sha512.New()
	h.Write(priv[:32])
	digest := h.Sum(nil)

	var curveKey [32]byte
	copy(curveKey[:], digest)

	// Aplicar clamping según la especificación de Curve25519.
	curveKey[0] &= 248
	curveKey[31] &= 127
	curveKey[31] |= 64

	return curveKey
}
// <<-- FIN: FUNCIÓN CORREGIDA -->>

func publicKeyToCurve25519(pub ed25519.PublicKey) ([32]byte, bool) {
	p, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		return [32]byte{}, false
	}

	montgomerySlice := p.BytesMontgomery()
	var montgomeryArray [32]byte
	copy(montgomeryArray[:], montgomerySlice)

	return montgomeryArray, true
}
