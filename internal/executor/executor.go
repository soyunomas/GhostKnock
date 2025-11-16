// El paquete executor se encarga de ejecutar comandos del sistema de forma segura.
package executor

import (
	"bytes"
	"fmt"
	"log/slog" // <<-- NUEVA IMPORTACIÓN
	"net"
	"os/exec"
	"text/template"
	"time"

	"github.com/your-org/ghostknock/internal/config"
)

// Execute procesa una acción, la ejecuta y, si está configurado, programa su reversión.
func Execute(action config.Action, sourceIP net.IP) error {
	slog.Debug("Ejecutando acción", "source_ip", sourceIP.String())

	// Ejecutar el comando principal.
	if err := runCommand("main", action.Command, sourceIP); err != nil {
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
	slog.Info(
		"Programando reversión de acción",
		"source_ip", sourceIP.String(),
		"delay", delay.String(),
	)
	time.Sleep(delay)

	slog.Info("Ejecutando reversión", "source_ip", sourceIP.String())
	if err := runCommand("revert", action.RevertCommand, sourceIP); err != nil {
		// Logueamos el error pero no podemos hacer mucho más, ya que estamos en una goroutine.
		slog.Error(
			"Falló la ejecución del comando de reversión",
			"source_ip", sourceIP.String(),
			"error", err,
		)
	}
}

// runCommand es el núcleo de la ejecución segura.
func runCommand(commandType, commandTemplate string, sourceIP net.IP) error {
	templateData := struct {
		SourceIP string
	}{
		SourceIP: sourceIP.String(),
	}

	tmpl, err := template.New("cmd").Parse(commandTemplate)
	if err != nil {
		return fmt.Errorf("error interno al parsear la plantilla de comando: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return fmt.Errorf("error interno al ejecutar la plantilla de comando: %w", err)
	}
	finalCommand := buf.String()

	cmd := exec.Command("/bin/sh", "-c", finalCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Info("Ejecutando comando en el shell",
		"type", commandType,
		"command", finalCommand,
		"source_ip", sourceIP.String(),
	)

	err = cmd.Run()

	// Registrar siempre la salida para una depuración completa.
	if stdout.Len() > 0 {
		slog.Debug("Comando ejecutado (stdout)", "type", commandType, "output", stdout.String())
	}
	if stderr.Len() > 0 {
		// La salida de error estándar se registra como un aviso.
		slog.Warn("Comando ejecutado (stderr)", "type", commandType, "output", stderr.String())
	}

	if err != nil {
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, stderr.String())
	}

	return nil
}
