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
	"os/user"
	"regexp" // <<-- NUEVA IMPORTACIÓN
	"strconv"
	"syscall"
	"text/template"
	"time"

	"github.com/your-org/ghostknock/internal/config"
)

// safeParamRegex define la lista blanca de caracteres permitidos en los parámetros.
// Empieza con un carácter alfanumérico, punto o guion bajo, y luego permite guiones medios.
// Esto previene inyecciones de comandos y que el valor sea interpretado como un flag.
var safeParamRegex = regexp.MustCompile(`^[a-zA-Z0-9._][a-zA-Z0-9._-]*$`)

// Execute procesa una acción, valida sus parámetros, la ejecuta y programa su reversión.
// Ahora acepta un mapa de parámetros sanitizados.
func Execute(action config.Action, sourceIP net.IP, params map[string]string) error {
	slog.Debug("Ejecutando acción", "source_ip", sourceIP.String())

	// Ejecutar el comando principal pasando los parámetros.
	if err := runCommand("main", action.Command, action.TimeoutSeconds, action.RunAsUser, sourceIP, params); err != nil {
		return fmt.Errorf("falló la ejecución del comando principal: %w", err)
	}

	// Si hay un comando de reversión y un retardo, programarlo.
	if action.RevertCommand != "" && action.RevertDelaySeconds > 0 {
		go scheduleRevert(action, sourceIP, params)
	}

	return nil
}

// scheduleRevert espera el tiempo especificado y luego ejecuta el comando de reversión.
func scheduleRevert(action config.Action, sourceIP net.IP, params map[string]string) {
	delay := time.Duration(action.RevertDelaySeconds) * time.Second
	slog.Info(
		"Programando reversión de acción",
		"source_ip", sourceIP.String(),
		"delay", delay.String(),
	)
	time.Sleep(delay)

	slog.Info("Ejecutando reversión", "source_ip", sourceIP.String())
	// La reversión también recibe los parámetros (ej. para cerrar el puerto a una IP específica enviada como param).
	if err := runCommand("revert", action.RevertCommand, action.TimeoutSeconds, action.RunAsUser, sourceIP, params); err != nil {
		slog.Error(
			"Falló la ejecución del comando de reversión",
			"source_ip", sourceIP.String(),
			"error", err,
		)
	}
}

// runCommand es el núcleo de la ejecución segura.
func runCommand(commandType, commandTemplate string, timeoutSeconds int, runAsUser string, sourceIP net.IP, params map[string]string) error {
	// 1. VALIDACIÓN DE SEGURIDAD DE PARÁMETROS (Sanitización Estricta)
	if len(params) > 0 {
		for key, value := range params {
			if !safeParamRegex.MatchString(value) {
				return fmt.Errorf("SEGURIDAD: El valor del parámetro '%s' contiene caracteres inválidos o empieza con un guion. Solo se permiten [a-zA-Z0-9._-] y no puede empezar con '-'", key)
			}
			// Validación redundante pero explícita contra path traversal relativo.
			if value == ".." {
				return fmt.Errorf("SEGURIDAD: Uso de '..' no permitido en parámetros")
			}
		}
	}

	// 2. PREPARACIÓN DE DATOS PARA LA PLANTILLA
	templateData := struct {
		SourceIP string
		Params   map[string]string
	}{
		SourceIP: sourceIP.String(),
		Params:   params,
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
		defer cancel()
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
		"run_as_user", runAsUser,
		"source_ip", sourceIP.String(),
	)

	err = cmd.Run()

	if stdout.Len() > 0 {
		slog.Debug("Comando ejecutado (stdout)", "type", commandType, "output", stdout.String())
	}
	if stderr.Len() > 0 {
		slog.Warn("Comando ejecutado (stderr)", "type", commandType, "output", stderr.String())
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			slog.Warn("Comando terminado por timeout",
				"type", commandType,
				"timeout_seconds", timeoutSeconds,
				"command", finalCommand,
			)
			return fmt.Errorf("el comando excedió el timeout de %d segundos", timeoutSeconds)
		}
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, stderr.String())
	}

	return nil
}
