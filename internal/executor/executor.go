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
	"text/template"
	"time"

	"github.com/soyunomas/ghostknock/internal/config"
)

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
	for key := range safe {
		if isSensitiveParam(key, sensitive) {
			safe[key] = "*****"
		}
	}
	return safe
}

// Execute procesa una acción, valida sus parámetros, la ejecuta y programa su reversión.
// MODIFICADO (v2.1): Usa templates pre-compilados en config.Action para evitar parsing runtime.
// MODIFICADO (seguridad): es método de Coordinator para que los post-hooks y las
// reversiones queden acotados por el semáforo de fondo y se esperen en el apagado.
func (c *Coordinator) Execute(action config.Action, user string, sourceIP net.IP, params map[string]string, globalHooks config.Hooks, daemonCfg config.Daemon) error {
	params = cloneParams(params)
	action.SensitiveParams = append([]string(nil), action.SensitiveParams...)

	if err := ValidateParams(params); err != nil {
		return err
	}
	if err := validateSensitiveParamNames(action.SensitiveParams); err != nil {
		return err
	}
	requiredParams, err := requiredParamsForTemplate(action.CommandTmpl)
	if err != nil {
		return fmt.Errorf("template principal inseguro: %w", err)
	}
	if err := validateRequiredParams(requiredParams, params); err != nil {
		return err
	}
	if action.RevertCommand != "" {
		revertRequiredParams, err := requiredParamsForTemplate(action.RevertCommandTmpl)
		if err != nil {
			return fmt.Errorf("template de reversión inseguro: %w", err)
		}
		if err := validateRequiredParams(revertRequiredParams, params); err != nil {
			return err
		}
	}

	// Usamos una versión sanitizada de los parámetros para el log de debug interno
	safeParams := redactParams(params, action.SensitiveParams)
	slog.Debug("Iniciando flujo de ejecución", "user", user, "source_ip", sourceIP.String(), "params", safeParams)

	// --- 1. PREPARAR CONTEXTO DE HOOKS ---
	hCtx := HookContext{
		User:            user,
		ActionID:        "unknown_id", // La struct Action actual no tiene el ID, pero el contexto global sí.
		SourceIP:        sourceIP.String(),
		Params:          params,
		SensitiveParams: action.SensitiveParams,
	}

	// --- 2. GLOBAL PRE-HOOK (Bloqueante) ---
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
	// Pasamos el Template pre-compilado en lugar de parsearlo aquí.
	cmdErr := runCommand("main", action.Command, action.CommandTmpl, action.TimeoutSeconds, action.RunAsUser, sourceIP, params, action.SensitiveParams, daemonCfg.ShellPath, daemonCfg.ShellFlag)

	// --- 5. DETERMINAR ESTADO PARA POST-HOOKS ---
	status := "success"
	errMsg := ""
	if cmdErr != nil {
		status = "error"
		errMsg = cmdErr.Error()
	}
	hCtx.Status = status
	hCtx.ErrorMsg = errMsg

	// --- 6. ACTION POST-HOOK (Fire-and-Forget, acotado y rastreado) ---
	if action.PostHook != "" {
		hCtxPost := hCtx
		hCtxPost.Stage = "action_post"
		c.goBounded(func() { _ = RunHook(action.PostHook, hCtxPost) })
	}

	// --- 7. GLOBAL POST-HOOKS (Fire-and-Forget, acotados y rastreados) ---
	if status == "success" {
		if globalHooks.OnSuccess != "" {
			hCtxGlobal := hCtx
			hCtxGlobal.Stage = "global_success"
			c.goBounded(func() { _ = RunHook(globalHooks.OnSuccess, hCtxGlobal) })
		}
	} else {
		if globalHooks.OnError != "" {
			hCtxGlobal := hCtx
			hCtxGlobal.Stage = "global_error"
			c.goBounded(func() { _ = RunHook(globalHooks.OnError, hCtxGlobal) })
		}
	}

	// --- 8. PROGRAMAR REVERSIÓN (rastreada; la espera y el comando se acotan) ---
	if action.RevertCommand != "" && action.RevertDelaySeconds > 0 && action.RevertCommandTmpl != nil {
		c.goTracked(func() { c.scheduleRevert(action, user, sourceIP, params, globalHooks, daemonCfg) })
	}

	if cmdErr != nil {
		return fmt.Errorf("falló la ejecución del comando principal: %w", cmdErr)
	}

	return nil
}

// scheduleRevert espera el tiempo especificado y luego ejecuta el comando de reversión.
// MODIFICADO (seguridad): es método de Coordinator. La espera es cancelable en el
// apagado (c.sleep) y el comando de reversión se ejecuta acotado por el semáforo de
// fondo (c.runBounded), evitando reversiones no acotadas como root.
func (c *Coordinator) scheduleRevert(action config.Action, user string, sourceIP net.IP, params map[string]string, globalHooks config.Hooks, daemonCfg config.Daemon) {
	delay := time.Duration(action.RevertDelaySeconds) * time.Second
	slog.Info(
		"Programando reversión de acción",
		"source_ip", sourceIP.String(),
		"delay", delay.String(),
	)
	c.sleep(delay)

	slog.Info("Ejecutando reversión", "source_ip", sourceIP.String())

	// Ejecutar comando de reversión usando el shell configurado y el template compilado.
	// Acotado por el semáforo de fondo para preservar el límite anti-forkbomb.
	var err error
	c.runBounded(func() {
		err = runCommand("revert", action.RevertCommand, action.RevertCommandTmpl, action.TimeoutSeconds, action.RunAsUser, sourceIP, params, action.SensitiveParams, daemonCfg.ShellPath, daemonCfg.ShellFlag)
	})

	if err != nil {
		slog.Error(
			"Falló la ejecución del comando de reversión",
			"source_ip", sourceIP.String(),
			"error", err,
		)
	}

	// --- HOOKS DE REVERSIÓN ---
	status := "success"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}

	hCtx := HookContext{
		User:            user,
		ActionID:        "revert_unknown",
		SourceIP:        sourceIP.String(),
		Params:          params,
		SensitiveParams: action.SensitiveParams,
		Status:          status,
		ErrorMsg:        errMsg,
	}

	// 1. Action Revert Hook
	if action.RevertHook != "" {
		hCtx.Stage = "action_revert"
		RunHook(action.RevertHook, hCtx)
	}

	// 2. Global Revert Hook
	if globalHooks.OnRevert != "" {
		hCtx.Stage = "global_revert"
		RunHook(globalHooks.OnRevert, hCtx)
	}
}

// runCommand es el núcleo de la ejecución segura.
// MODIFICADO (v2.1): Acepta `tmpl *template.Template` ya compilado.
// Mantenemos `commandTemplate` string solo para logs y validación de regex.
func runCommand(commandType, commandTemplate string, tmpl *template.Template, timeoutSeconds int, runAsUser string, sourceIP net.IP, params map[string]string, sensitiveParams []string, shellPath string, shellFlag string) error {
	if tmpl == nil {
		return fmt.Errorf("error interno: template no compilado para %s", commandType)
	}

	// 1. DEFENSA EN PROFUNDIDAD: la validación principal ocurre en Execute,
	// antes de hooks y logs, pero runCommand también puede usarse internamente.
	if err := ValidateParams(params); err != nil {
		return err
	}

	// 2. VERIFICAR PARÁMETROS REQUERIDOS. Execute hace esta comprobación antes
	// de hooks; se repite aquí como defensa en profundidad.
	requiredParams, err := requiredParamsForTemplate(tmpl)
	if err != nil {
		return fmt.Errorf("template inseguro: %w", err)
	}
	if err := validateRequiredParams(requiredParams, params); err != nil {
		return err
	}

	// 3. PREPARACIÓN DE DATOS PARA LA PLANTILLA
	templateData := struct {
		SourceIP string
		Params   map[string]string
	}{
		SourceIP: sourceIP.String(),
		Params:   params,
	}

	// --- OPTIMIZACIÓN: EJECUCIÓN DIRECTA DEL TEMPLATE ---
	// Ya no hacemos template.New(...).Parse(...). Usamos el tmpl inyectado.
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

	// Ejecución del Shell
	cmd := exec.CommandContext(ctx, shellPath, shellFlag, finalCommand)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runAsUser != "" {
		u, err := user.Lookup(runAsUser)
		if err != nil {
			return fmt.Errorf("error crítico en tiempo de ejecución: no se pudo encontrar el usuario '%s': %w", runAsUser, err)
		}

		// --- MODIFICACIÓN QUIRÚRGICA AQUÍ ---
		// Reemplazamos la lógica directa de syscall por la función abstracta setCredentials
		// Esto permite que compile en Windows (usando el stub) y en Linux (usando syscall real)
		if err := setCredentials(cmd, u); err != nil {
			return fmt.Errorf("error al establecer credenciales para '%s': %w", runAsUser, err)
		}
		// ------------------------------------
	}

	// --- LÓGICA DE LOGGING SEGURO ---
	logCommandStr := finalCommand
	if len(sensitiveParams) > 0 {
		logCommandStr = fmt.Sprintf("[REDACTADO] %s (Valores ocultos por sensitive_params)", commandTemplate)
	}

	slog.Info("Ejecutando comando en el shell",
		"type", commandType,
		"command", logCommandStr,
		"shell", shellPath,
		"timeout_seconds", timeoutSeconds,
		"run_as_user", runAsUser,
		"source_ip", sourceIP.String(),
	)

	err = cmd.Run()

	redactedStdout := redactText(stdout.String(), params, sensitiveParams)
	redactedStderr := redactText(stderr.String(), params, sensitiveParams)
	if redactedStdout != "" {
		slog.Debug("Comando ejecutado (stdout)", "type", commandType, "output", redactedStdout)
	}
	if redactedStderr != "" {
		slog.Warn("Comando ejecutado (stderr)", "type", commandType, "output", redactedStderr)
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			slog.Warn("Comando terminado por timeout",
				"type", commandType,
				"timeout_seconds", timeoutSeconds,
				"command", logCommandStr,
			)
			return fmt.Errorf("el comando excedió el timeout de %d segundos", timeoutSeconds)
		}
		return fmt.Errorf("el comando falló: %w. Stderr: %s", err, redactedStderr)
	}

	return nil
}
