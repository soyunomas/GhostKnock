// ghostknock-keygen es una utilidad para generar pares de claves criptográficas
// ed25519 para su uso con GhostKnock.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	privateKeyPerms = 0600
	publicKeyPerms  = 0644
	configDirPerms  = 0700 // Solo el propietario puede acceder a este directorio.
	defaultKeyFile  = "id_ed25519"
)

// fileExists comprueba si un archivo existe en la ruta dada.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	// os.IsNotExist es la forma idiomática de comprobar si el error se debe a que el archivo no existe.
	return !os.IsNotExist(err)
}

func main() {
	log.SetFlags(0)

	// 1. OBTENER LA RUTA DE CONFIGURACIÓN POR DEFECTO
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("FATAL: No se pudo determinar el directorio home del usuario: %v", err)
	}
	defaultPath := filepath.Join(homeDir, ".config", "ghostknock", defaultKeyFile)

	// 2. AÑADIMOS UN FLAG PARA EL ARCHIVO DE SALIDA CON UN NUEVO VALOR POR DEFECTO
	// El texto de ayuda ahora muestra la ruta por defecto, haciéndola más clara.
	outputFile := flag.String("o", defaultPath, "Ruta base para guardar el par de claves (ej. ~/.ssh/ghostknock_admin)")
	flag.Parse()

	privateKeyFile := *outputFile
	publicKeyFile := privateKeyFile + ".pub"

	// 3. IMPLEMENTAMOS LA COMPROBACIÓN DE SEGURIDAD ANTI-SOBRESCRITURA
	if fileExists(privateKeyFile) || fileExists(publicKeyFile) {
		log.Fatalf(
			"FATAL: El archivo de clave '%s' o '%s' ya existe.\nPor seguridad, no se sobrescribirán. Por favor, elimínelos o elija otra ruta con el flag -o.",
			privateKeyFile,
			publicKeyFile,
		)
	}

	log.Printf("Generando un nuevo par de claves ed25519...")

	// 4. CREAR EL DIRECTORIO DE CONFIGURACIÓN SI NO EXISTE
	// os.MkdirAll es perfecto para esto: no hace nada si el directorio ya existe.
	keyDir := filepath.Dir(privateKeyFile)
	if err := os.MkdirAll(keyDir, configDirPerms); err != nil {
		log.Fatalf("FATAL: No se pudo crear el directorio de configuración en '%s': %v", keyDir, err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("Error fatal al generar el par de claves: %v", err)
	}

	err = os.WriteFile(privateKeyFile, privateKey, privateKeyPerms)
	if err != nil {
		log.Fatalf("Error al guardar la clave privada en '%s': %v", privateKeyFile, err)
	}
	log.Printf("Clave privada guardada de forma segura en: %s", privateKeyFile)

	err = os.WriteFile(publicKeyFile, publicKey, publicKeyPerms)
	if err != nil {
		// Si falla la escritura de la clave pública, eliminamos la privada para no dejar un estado inconsistente.
		_ = os.Remove(privateKeyFile)
		log.Fatalf("Error al guardar la clave pública en '%s': %v", publicKeyFile, err)
	}
	log.Printf("Clave pública guardada en: %s", publicKeyFile)

	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey)

	fmt.Println("\n---")
	fmt.Println("¡Claves generadas con éxito!")
	fmt.Println("Añada la siguiente clave pública a la sección 'users' de su archivo config.yaml en el servidor:")
	fmt.Printf("\n%s\n\n", publicKeyB64)
}
