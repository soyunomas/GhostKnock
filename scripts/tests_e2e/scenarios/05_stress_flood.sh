#!/bin/bash
# FAMILY: 05_stress_flood
# Objetivo: Estabilidad del servidor ante paquetes malformados (Garbage/Fuzzing).

generate_keys

# Configuración
cat <<EOF > "$WORK_DIR/config_05.yaml"
server_private_key_path: "$WORK_DIR/keys/server_key"
listener:
  interface: "lo"
  port: 45004
  listen_ip: "127.0.0.1"
logging:
  log_level: "info"
  log_output: "stdout"
# Aumentamos un poco el burst para la prueba de recuperación
security:
  rate_limit_burst: 5
daemon:
  pid_file: "$WORK_DIR/pid_05.pid"
users:
  - name: "survivor"
    public_key: "$CLIENT_PUB_B64"
    actions: ["status"]
actions:
  "status":
    command: 'echo ALIVE > $WORK_DIR/alive.txt'
    cooldown_seconds: 0
EOF

start_daemon "$(cat $WORK_DIR/config_05.yaml)" "05_stress_flood"

# ==============================================================================
# TEST 1: Garbage Flood
# ==============================================================================
echo ">> [Test 1] Enviando 5000 paquetes de basura aleatoria UDP..."

(
    for i in {1..5000}; do
        head -c 64 /dev/urandom > /dev/udp/127.0.0.1/45004 2>/dev/null
    done
) &
FLOOD_PID=$!
wait $FLOOD_PID

echo "   ...Ataque finalizado."

if kill -0 "$CURRENT_DAEMON_PID" 2>/dev/null; then
    echo -e "  ${C_GREEN}[PASS] Stability: Daemon process is still running${C_NC}"
    echo "[PASS] Stability: Daemon process is still running" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Stability: Daemon CRASHED under flood${C_NC}"
    echo "[FAIL] Stability: Daemon CRASHED under flood" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
    exit 1
fi

# ==============================================================================
# TEST 2: Recuperación (Health Check)
# ==============================================================================
echo ">> [Test 2] Verificando operatividad post-ataque..."

# FIX CRÍTICO: Esperar a que el Rate Limiter se enfríe.
# El flood llenó el bucket. Si intentamos inmediatamente, nos bloqueará.
echo "   ...Esperando enfriamiento del Rate Limiter (4s)..."
sleep 4

rm -f "$WORK_DIR/alive.txt"
run_client -port 45004 -action "status"
sleep 1

if grep -q "ALIVE" "$WORK_DIR/alive.txt" 2>/dev/null; then
    echo -e "  ${C_GREEN}[PASS] Recovery: Daemon processed valid packet after flood${C_NC}"
    echo "[PASS] Recovery: Daemon processed valid packet after flood" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Recovery: Daemon failed to process packet${C_NC}"
    echo "         (Check logs for rate_limit_exceeded or crash)"
    echo "[FAIL] Recovery: Daemon failed to process packet" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
fi

stop_daemon
