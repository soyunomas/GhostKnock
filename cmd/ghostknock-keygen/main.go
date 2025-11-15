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
)

const (
	privateKeyPerms = 0600
	publicKeyPerms  = 0644
)

// fileExists comprueba si un archivo existe en la ruta dada.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	// os.IsNotExist es la forma idiomática de comprobar si el error se debe a que el archivo no existe.
	return !os.IsNotExist(err)
}

func main() {
	log.SetFlags(0)
	
	// 1. AÑADIMOS UN FLAG PARA EL ARCHIVO DE SALIDA
	// El valor por defecto sigue siendo "id_ed25519" para mantener la simplicidad del caso de uso básico.
	outputFile := flag.String("o", "id_ed25519", "Ruta base para guardar el par de claves (ej. ~/.ssh/ghostknock_admin)")
	flag.Parse()

	privateKeyFile := *outputFile
	publicKeyFile := privateKeyFile + ".pub"

	// 2. IMPLEMENTAMOS LA COMPROBACIÓN DE SEGURIDAD ANTI-SOBRESCRITURA
	if fileExists(privateKeyFile) || fileExists(publicKeyFile) {
		log.Fatalf(
			"FATAL: El archivo de clave '%s' o '%s' ya existe.\nPor seguridad, no se sobrescribirán. Por favor, elimínelos o elija otra ruta con el flag -o.",
			privateKeyFile,
			publicKeyFile,
		)
	}

	log.Printf("Generando un nuevo par de claves ed25519 en '%s' y '%s'...", privateKeyFile, publicKeyFile)

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
