#!/bin/bash
# FAMILY: 01_crypto_auth
# Objetivo: Verificar autenticación, firmas y rechazo de claves inválidas.

# 1. Preparar claves nuevas para este escenario
generate_keys

# 2. Definir Configuración (YAML dinámico)
# Usamos puerto 45000 para testing
cat <<EOF > "$WORK_DIR/config_01.yaml"
server_private_key_path: "$WORK_DIR/keys/server_key"
listener:
  interface: "lo"
  port: 45000
  listen_ip: "127.0.0.1"
logging:
  log_level: "debug"
  log_output: "stdout" # Redirigimos al log del script común
daemon:
  pid_file: "$WORK_DIR/pid_01.pid"
users:
  - name: "tester_legit"
    public_key: "$CLIENT_PUB_B64"
    actions:
      - "ping-check"
actions:
  "ping-check":
    command: "echo PONG > $WORK_DIR/pong.txt"
    cooldown_seconds: 0
EOF

# 3. Arrancar Demonio (pasando el contenido del yaml y el nombre del escenario)
start_daemon "$(cat $WORK_DIR/config_01.yaml)" "01_crypto_auth"

# --- TEST 1: Knock Legítimo ---
echo ">> Ejecutando Knock Legítimo..."
run_client -action "ping-check"

# Verificaciones
sleep 0.5
assert_log "Knock válido recibido" "Auth: Valid Signature Accepted"
if [ -f "$WORK_DIR/pong.txt" ]; then
    echo -e "  ${C_GREEN}[PASS] Exec: Command output file created${C_NC}"
else
    echo -e "  ${C_RED}[FAIL] Exec: Command output file NOT created${C_NC}"
    ((TESTS_FAILED++))
fi

# --- TEST 2: Clave Incorrecta (Atacante) ---
echo ">> Ejecutando Knock de Atacante..."
# Usamos la clave del atacante explícitamente
run_client \
    -action "ping-check" \
    -key "$WORK_DIR/keys/attacker_key" 

# Verificaciones
sleep 0.5
# Buscamos mensaje de fallo de descifrado o firma
assert_log "invalid_signature_or_decryption_failed" "Auth: Attacker Rejected"

# --- TEST 3: Replay Attack (Simulado) ---
# Intentamos usar la misma clave válida inmediatamente (si no hubiera cooldown/replay window)
# Nota: La config tiene cooldown 0, pero el anti-replay cache debería saltar si es MUY rápido
# o si capturamos el paquete raw (difícil de simular solo con el cliente binario).
# Probaremos enviar un ActionID que no existe para ver si valida firma pero falla logica.

echo ">> Ejecutando Acción Inexistente (Firma válida)..."
run_client -action "fake-action"

sleep 0.5
assert_log "unauthorized_action" "Logic: Unauthorized Action ID Rejected"

# 4. Limpieza del escenario (automática por common.lib, pero bueno ponerlo)
stop_daemon
