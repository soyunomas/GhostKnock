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
	"regexp"
	"strconv"
	"syscall"
	"text/template"
	"time"

	"github.com/your-org/ghostknock/internal/config"
)

// safeParamRegex define la lista blanca de caracteres permitidos en los parámetros.
var safeParamRegex = regexp.MustCompile(`^[a-zA-Z0-9._][a-zA-Z0-9._-]*$`)

// templateParamRegex encuentra todas las instancias de {{.Params.key}} en una plantilla.
var templateParamRegex = regexp.MustCompile(`\{\{\.Params\.([a-zA-Z0-9_]+)\}\}`)

// redactParams crea una copia segura de los parámetros, ocultando los sensibles.
func redactParams(params map[string]string, sensitive []string) map[string]string {
	if len(sensitive) == 0 {
		return params
	}
	// Copiamos el mapa para no alterar el original
	safe := make(map[string]string, len(params))
	for k, v := range params {
		safe[k] = v
	}

	// Censuramos los campos sensibles
	for _, key := range sensitive {
		if _, exists := safe[key]; exists {
			safe[key] = "*****"
		}
	}
	return safe
}

// Execute procesa una acción, valida sus parámetros, la ejecuta y programa su reversión.
// MODIFICADO (v2.2): Ahora acepta 'user' y 'globalHooks' para el sistema de Hooks.
func Execute(action config.Action, user string, sourceIP net.IP, params map[string]string, globalHooks config.Hooks) error {
	// Usamos una versión sanitizada de los parámetros para el log de debug interno
	safeParams := redactParams(params, action.SensitiveParams)
	slog.Debug("Iniciando flujo de ejecución", "user", user, "source_ip", sourceIP.String(), "params", safeParams)

	// --- 1. PREPARAR CONTEXTO DE HOOKS ---
	// Nota: Los hooks reciben los parámetros CRUDOS en variables de entorno (GK_PARAM_X).
	// Se asume que el script del hook es seguro y confiable.
	hCtx := HookContext{
		User:     user,
		ActionID: "unknown_id", // La struct Action actual no tiene el ID, pero el contexto global sí. (Mejora futura: pasar ID explícito)
		SourceIP: sourceIP.String(),
		Params:   params,
	}

	// --- 2. GLOBAL PRE-HOOK (Bloqueante) ---
	// Si falla, abortamos todo.
	hCtx.Stage = "global_pre"
	if err := RunHook(globalHooks.PreExecute, hCtx); err != nil {
		slog.Warn("Ejecución cancelada por Global Pre-Hook", "user", user)
		return fmt.Errorf("action cancelled by Global Pre-Hook")
	}

	// --- 3. ACTION PRE-HOOK (Bloqueante) ---
	hCtx.Stage = "action_pre"
	if err := RunHook(action.PreHook, hCtx); err != nil {
		slog.Warn("Ejecución cancelada por Action Pre-Hook", "user", user)
		return fmt.Errorf("action cancelled by Action Pre-Hook")
	}

	// --- 4. EJECUCIÓN DEL COMANDO PRINCIPAL ---
	cmdErr := runCommand("main", action.Command, action.TimeoutSeconds, action.RunAsUser, sourceIP, params, action.SensitiveParams)

	// --- 5. DETERMINAR ESTADO PARA POST-HOOKS ---
	status := "success"
	errMsg := ""
	if cmdErr != nil {
		status = "error"
		errMsg = cmdErr.Error()
	}
	hCtx.Status = status
	hCtx.ErrorMsg = errMsg

	// --- 6. ACTION POST-HOOK (Fire-and-Forget) ---
	if action.PostHook != "" {
		// Lanzamos en goroutine para no retrasar la respuesta o el siguiente paso
		hCtxPost := hCtx
		hCtxPost.Stage = "action_post"
		go RunHook(action.PostHook, hCtxPost)
	}

	// --- 7. GLOBAL POST-HOOKS (Fire-and-Forget) ---
	if status == "success" {
		if globalHooks.OnSuccess != "" {
			hCtxGlobal := hCtx
			hCtxGlobal.Stage = "global_success"
			go RunHook(globalHooks.OnSuccess, hCtxGlobal)
		}
	} else {
		if globalHooks.OnError != "" {
			hCtxGlobal := hCtx
			hCtxGlobal.Stage = "global_error"
			go RunHook(globalHooks.OnError, hCtxGlobal)
		}
	}

	// --- 8. PROGRAMAR REVERSIÓN ---
	// Si hay un comando de reversión y un retardo, programarlo.
	if action.RevertCommand != "" && action.RevertDelaySeconds > 0 {
		// Pasamos los hooks y el usuario a la rutina de reversión
		go scheduleRevert(action, user, sourceIP, params, globalHooks)
	}

	if cmdErr != nil {
		return fmt.Errorf("falló la ejecución del comando principal: %w", cmdErr)
	}

	return nil
}

// scheduleRevert espera el tiempo especificado y luego ejecuta el comando de reversión.
// MODIFICADO (v2.2): Soporte para Hooks de reversión.
func scheduleRevert(action config.Action, user string, sourceIP net.IP, params map[string]string, globalHooks config.Hooks) {
	delay := time.Duration(action.RevertDelaySeconds) * time.Second
	slog.Info(
		"Programando reversión de acción",
		"source_ip", sourceIP.String(),
		"delay", delay.String(),
	)
	time.Sleep(delay)

	slog.Info("Ejecutando reversión", "source_ip", sourceIP.String())
	
	// Ejecutar comando de reversión
	err := runCommand("revert", action.RevertCommand, action.TimeoutSeconds, action.RunAsUser, sourceIP, params, action.SensitiveParams)
	
	if err != nil {
		slog.Error(
			"Falló la ejecución del comando de reversión",
			"source_ip", sourceIP.String(),
			"error", err,
		)
	}

	// --- HOOKS DE REVERSIÓN ---
	
	// Preparamos el contexto
	status := "success"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}

	hCtx := HookContext{
		User:     user,
		ActionID: "revert_unknown", // Idem que en Execute
		SourceIP: sourceIP.String(),
		Params:   params,
		Status:   status,
		ErrorMsg: errMsg,
	}

	// 1. Action Revert Hook
	if action.RevertHook != "" {
		hCtx.Stage = "action_revert"
		// Ejecutamos síncronamente aquí porque ya estamos en una goroutine separada
		RunHook(action.RevertHook, hCtx)
	}

	// 2. Global Revert Hook
	if globalHooks.OnRevert != "" {
		hCtx.Stage = "global_revert"
		RunHook(globalHooks.OnRevert, hCtx)
	}
}

// runCommand es el núcleo de la ejecución segura.
func runCommand(commandType, commandTemplate string, timeoutSeconds int, runAsUser string, sourceIP net.IP, params map[string]string, sensitiveParams []string) error {
	// 1. VALIDACIÓN DE SEGURIDAD DE PARÁMETROS (Sanitización Estricta)
	for key, value := range params {
		if !safeParamRegex.MatchString(value) {
			return fmt.Errorf("SEGURIDAD: El valor del parámetro '%s' contiene caracteres inválidos o empieza con un guion. Solo se permiten [a-zA-Z0-9._-] y no puede empezar con '-'", key)
		}
		if value == ".." {
			return fmt.Errorf("SEGURIDAD: Uso de '..' no permitido en parámetros")
		}
	}

	// 2. VERIFICAR QUE TODOS LOS PARÁMETROS REQUERIDOS EN LA PLANTILLA ESTÁN PRESENTES
	requiredParams := templateParamRegex.FindAllStringSubmatch(commandTemplate, -1)
	for _, match := range requiredParams {
		paramName := match[1] // El primer grupo de captura es el nombre del parámetro.
		if _, ok := params[paramName]; !ok {
			return fmt.Errorf("SEGURIDAD: El comando requiere el parámetro '%s', pero no fue proporcionado por el cliente", paramName)
		}
	}

	// 3. PREPARACIÓN DE DATOS PARA LA PLANTILLA
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

	// --- LÓGICA DE LOGGING SEGURO ---
	// Si hay parámetros sensibles, NO mostramos el comando final expandido en los logs.
	logCommandStr := finalCommand
	if len(sensitiveParams) > 0 {
		logCommandStr = fmt.Sprintf("[REDACTADO] %s (Valores ocultos por sensitive_params)", commandTemplate)
	}

	slog.Info("Ejecutando comando en el shell",
		"type", commandType,
		"command", logCommandStr, // Usamos la versión segura
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
				"command", logCommandStr, // Log seguro
			)
			return fmt.Errorf("el comando excedió el timeout de %d segundos", timeoutSeconds)
		}
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, stderr.String())
	}

	return nil
}
