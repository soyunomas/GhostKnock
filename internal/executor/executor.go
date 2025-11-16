// El paquete executor se encarga de ejecutar comandos del sistema de forma segura.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"os/user" // <<-- NUEVA IMPORTACIÓN
	"strconv" // <<-- NUEVA IMPORTACIÓN
	"syscall" // <<-- NUEVA IMPORTACIÓN
	"text/template"
	"time"

	"github.com/your-org/ghostknock/internal/config"
)

// Execute procesa una acción, la ejecuta y, si está configurado, programa su reversión.
func Execute(action config.Action, sourceIP net.IP) error {
	slog.Debug("Ejecutando acción", "source_ip", sourceIP.String())

	// Ejecutar el comando principal.
	if err := runCommand("main", action.Command, action.TimeoutSeconds, action.RunAsUser, sourceIP); err != nil {
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
	if err := runCommand("revert", action.RevertCommand, action.TimeoutSeconds, action.RunAsUser, sourceIP); err != nil {
		// Logueamos el error pero no podemos hacer mucho más, ya que estamos en una goroutine.
		slog.Error(
			"Falló la ejecución del comando de reversión",
			"source_ip", sourceIP.String(),
			"error", err,
		)
	}
}

// runCommand es el núcleo de la ejecución segura.
func runCommand(commandType, commandTemplate string, timeoutSeconds int, runAsUser string, sourceIP net.IP) error {
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

	// --- LÓGICA DE TIMEOUT CON CONTEXT ---
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		timeoutDuration := time.Duration(timeoutSeconds) * time.Second
		ctx, cancel = context.WithTimeout(ctx, timeoutDuration)
		defer cancel() // Asegura que los recursos del contexto se liberen.
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", finalCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// --- LÓGICA DE EJECUCIÓN CON PRIVILEGIOS REDUCIDOS ---
	if runAsUser != "" {
		u, err := user.Lookup(runAsUser)
		if err != nil {
			return fmt.Errorf("error crítico en tiempo de ejecución: no se pudo encontrar el usuario '%s': %w", runAsUser, err)
		}

		uid, err := strconv.ParseUint(u.Uid, 10, 32)
		if err != nil {
			return fmt.Errorf("no se pudo parsear el UID '%s' para el usuario '%s': %w", u.Uid, runAsUser, err)
		}

		gid, err := strconv.ParseUint(u.Gid, 10, 32)
		if err != nil {
			return fmt.Errorf("no se pudo parsear el GID '%s' para el usuario '%s': %w", u.Gid, runAsUser, err)
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	}
	// --------------------------------------------------------

	slog.Info("Ejecutando comando en el shell",
		"type", commandType,
		"command", finalCommand,
		"timeout_seconds", timeoutSeconds,
		"run_as_user", runAsUser, // <<-- Logueo del usuario
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

	// --- MANEJO DE ERRORES MEJORADO ---
	if err != nil {
		// Comprobar si el error fue causado por el timeout de nuestro contexto.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			slog.Warn("Comando terminado por timeout",
				"type", commandType,
				"timeout_seconds", timeoutSeconds,
				"command", finalCommand,
			)
			// Devolvemos un error específico para el timeout.
			return fmt.Errorf("el comando excedió el timeout de %d segundos", timeoutSeconds)
		}
		// Si es otro tipo de error, lo reportamos como tal.
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, stderr.String())
	}

	return nil
}
