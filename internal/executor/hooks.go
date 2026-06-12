// El paquete executor se encarga de ejecutar comandos del sistema de forma segura.
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HookContext define el contexto de datos que se inyectará al script del hook como variables de entorno.
type HookContext struct {
	User            string
	ActionID        string
	SourceIP        string
	Stage           string            // Ej: "global_pre", "action_post", "global_revert"
	Status          string            // "success", "error"
	ErrorMsg        string            // Detalle del error si Status == "error"
	Params          map[string]string // Parámetros crudos del knock
	SensitiveParams []string          // Claves cuyos valores deben ocultarse en logs
}

// RunHook ejecuta un script externo inyectando el contexto mediante variables de entorno.
// Impone un timeout estricto de 5 segundos.
func RunHook(scriptPath string, ctx HookContext) error {
	if scriptPath == "" {
		return nil
	}
	if err := ValidateParams(ctx.Params); err != nil {
		return err
	}

	// Timeout de seguridad: 5 segundos. Un hook no debe colgar el sistema.
	ctxt, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxt, scriptPath)

	// HERENCIA DE ENTORNO + INYECCIÓN
	// Es crucial incluir os.Environ() para que el script tenga acceso al PATH y herramientas del sistema.
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		"GK_USER="+ctx.User,
		"GK_ACTION="+ctx.ActionID,
		"GK_IP="+ctx.SourceIP,
		"GK_STAGE="+ctx.Stage,
		"GK_STATUS="+ctx.Status,
		"GK_ERROR_MSG="+ctx.ErrorMsg,
	)

	// Inyectar también los Params del usuario como GK_PARAM_KEY=val
	for k, v := range ctx.Params {
		// Convertimos la clave a mayúsculas para convención estándar de variables de entorno
		safeKey := "GK_PARAM_" + strings.ToUpper(k)
		cmd.Env = append(cmd.Env, safeKey+"="+v)
	}

	slog.Debug("Ejecutando Hook", "stage", ctx.Stage, "script", scriptPath)

	// Ejecutamos y capturamos salida combinada (stdout + stderr)
	output, err := cmd.CombinedOutput()
	safeOutput := redactText(string(output), ctx.Params, ctx.SensitiveParams)

	if err != nil {
		// Comprobamos si falló por timeout
		if ctxt.Err() == context.DeadlineExceeded {
			slog.Error("Hook finalizado forzosamente por Timeout (5s)",
				"stage", ctx.Stage,
				"script", scriptPath)
			return fmt.Errorf("hook timeout exceeded")
		}

		// Fallo de ejecución normal (exit code != 0) o archivo no encontrado
		slog.Error("Hook fallido",
			"stage", ctx.Stage,
			"script", scriptPath,
			"error", err,
			"output", safeOutput)
		return fmt.Errorf("hook execution failed: %w", err)
	}

	// Si hay salida en stdout/stderr y el hook fue exitoso, la registramos en nivel Debug
	if safeOutput != "" {
		slog.Debug("Hook output", "stage", ctx.Stage, "output", safeOutput)
	}

	return nil
}
