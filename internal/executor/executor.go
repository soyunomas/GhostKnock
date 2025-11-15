// El paquete executor se encarga de ejecutar comandos del sistema de forma segura.
package executor

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os/exec"
	"text/template"
	"time"

	"github.com/your-org/ghostknock/internal/config"
)

// Execute procesa una acción, la ejecuta y, si está configurado, programa su reversión.
func Execute(action config.Action, sourceIP net.IP) error {
	log.Printf("[Executor] Ejecutando acción para IP %s", sourceIP)

	// Ejecutar el comando principal.
	if err := runCommand(action.Command, sourceIP); err != nil {
		// Envolvemos el error para dar más contexto en los logs.
		return fmt.Errorf("falló la ejecución del comando principal: %w", err)
	}

	// Si hay un comando de reversión y un retardo, programarlo en una nueva goroutine.
	if action.RevertCommand != "" && action.RevertDelaySeconds > 0 {
		go scheduleRevert(action, sourceIP)
	}

	return nil
}

// scheduleRevert espera el tiempo especificado y luego ejecuta el comando de reversión.
func scheduleRevert(action config.Action, sourceIP net.IP) {
	delay := time.Duration(action.RevertDelaySeconds) * time.Second
	log.Printf("[Executor] Programando reversión para la IP %s en %s", sourceIP, delay)
	time.Sleep(delay)

	log.Printf("[Executor] Ejecutando reversión para la IP %s", sourceIP)
	if err := runCommand(action.RevertCommand, sourceIP); err != nil {
		// Logueamos el error pero no podemos hacer mucho más, ya que estamos en una goroutine.
		log.Printf("[ERROR] Falló la ejecución del comando de reversión para la IP %s: %v", sourceIP, err)
	}
}

// runCommand es el núcleo de la ejecución segura. Utiliza templates para
// construir el comando y lo ejecuta a través de un shell para soportar
// redirecciones y otras características.
func runCommand(commandTemplate string, sourceIP net.IP) error {
	// 1. Preparar los datos para la plantilla.
	templateData := struct {
		SourceIP string
	}{
		SourceIP: sourceIP.String(),
	}

	// 2. Crear y ejecutar la plantilla para construir el comando final.
	tmpl, err := template.New("cmd").Parse(commandTemplate)
	if err != nil {
		return fmt.Errorf("error interno al parsear la plantilla de comando: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return fmt.Errorf("error interno al ejecutar la plantilla de comando: %w", err)
	}
	finalCommand := buf.String()

	// 3. **CAMBIO CLAVE**: Ejecutar el comando a través de `/bin/sh -c`.
	// Esto permite el uso de tuberías (|), redirección (>, <) y otras
	// funcionalidades del shell en los comandos definidos en config.yaml.
	cmd := exec.Command("/bin/sh", "-c", finalCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("[Executor] Comando a ejecutar (via sh -c): %s", finalCommand)

	// 4. Ejecutar el comando.
	err = cmd.Run()

	// Registrar siempre la salida para una depuración completa.
	if stdout.Len() > 0 {
		log.Printf("[Executor] Salida (stdout): %s", stdout.String())
	}
	if stderr.Len() > 0 {
		log.Printf("[Executor] Error (stderr): %s", stderr.String())
	}

	if err != nil {
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, stderr.String())
	}

	return nil
}
