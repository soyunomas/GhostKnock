#!/bin/bash
# FAMILY: 03_network_policy
# Objetivo: Verificar listas blancas de IP (CIDR), rangos de red y el sistema Anti-DoS (Rate Limiting).

# 1. GESTIÓN DE CLAVES MULTI-USUARIO
echo ">> [Setup] Generando claves para múltiples perfiles de red..."
mkdir -p "$WORK_DIR/keys"

gen_key_pair() {
    local name="$1"
    "$BIN_KEYGEN" -o "$WORK_DIR/keys/$name" > /dev/null 2>&1
    base64 -w 0 < "$WORK_DIR/keys/$name.pub"
}

KEY_EXACT=$(gen_key_pair "user_exact")   # Solo permite 127.0.0.1/32
KEY_WRONG=$(gen_key_pair "user_wrong")   # Solo permite 10.0.0.5/32
KEY_SUBNET=$(gen_key_pair "user_subnet") # Permite 127.0.0.0/8
KEY_ANY=$(gen_key_pair "user_any")       # Permite todo

# 2. CONFIGURACIÓN
cat <<EOF > "$WORK_DIR/config_03.yaml"
server_private_key_path: "$WORK_DIR/keys/server_key"
listener:
  interface: "lo"
  port: 45002
  listen_ip: "127.0.0.1"
logging:
  log_level: "debug"
  log_output: "stdout"

# SEGURIDAD ESTRICTA PARA EL TEST DE ESTRÉS
security:
  rate_limit_per_second: 1.0
  rate_limit_burst: 3

daemon:
  pid_file: "$WORK_DIR/pid_03.pid"

users:
  - name: "usr_exact_ip"
    public_key: "$KEY_EXACT"
    actions: ["ping"]
    source_ips: ["127.0.0.1/32"]

  - name: "usr_wrong_ip"
    public_key: "$KEY_WRONG"
    actions: ["ping"]
    source_ips: ["10.0.0.5/32"]

  - name: "usr_subnet"
    public_key: "$KEY_SUBNET"
    actions: ["ping"]
    source_ips: ["127.0.0.0/8"]

  - name: "usr_any"
    public_key: "$KEY_ANY"
    actions: ["ping"]

actions:
  "ping":
    command: 'echo PONG'
    cooldown_seconds: 0
EOF

# 3. ARRANCAR DEMONIO
start_daemon "$(cat $WORK_DIR/config_03.yaml)" "03_network_policy"

# ==============================================================================
# TEST 1: CIDR Exacto (/32)
# ==============================================================================
echo ">> [Test 1] Validación de IP Exacta..."
run_client -port 45002 -key "$WORK_DIR/keys/user_exact" -action "ping"
sleep 0.5
assert_log "usr_exact_ip" "Network: Allowed specific IP" "true"

# ==============================================================================
# TEST 2: CIDR Incorrecto
# ==============================================================================
echo ">> [Test 2] Rechazo de IP No Autorizada..."
run_client -port 45002 -key "$WORK_DIR/keys/user_wrong" -action "ping"
sleep 0.5
if grep -q "unauthorized_source_ip" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Network: Blocked Unauthorized IP${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Network: Unauthorized IP was accepted!${C_NC}"
    ((TESTS_FAILED++))
fi

# ==============================================================================
# TEST 3: Validación de Subred
# ==============================================================================
echo ">> [Test 3] Validación de Rango de Subred..."
run_client -port 45002 -key "$WORK_DIR/keys/user_subnet" -action "ping"
sleep 0.5
assert_log "usr_subnet" "Network: Allowed via CIDR Subnet" "true"

# ==============================================================================
# TEST 4: Sin Restricciones
# ==============================================================================
echo ">> [Test 4] Usuario sin restricciones de IP..."
run_client -port 45002 -key "$WORK_DIR/keys/user_any" -action "ping"
sleep 0.5
assert_log "usr_any" "Network: Allowed Any IP" "true"

# ==============================================================================
# TEST 5: Rate Limiting (Anti-DoS)
# ==============================================================================
echo ">> [Test 5] Prueba de Rate Limit (Flood)..."
echo "   Config: Burst=3. Enviando 15 paquetes rápidos..."

# Bucle de ataque
for i in {1..15}; do
    "$BIN_CLIENT" \
        -host "127.0.0.1" \
        -port 45002 \
        -server-pubkey "$WORK_DIR/keys/server_key.pub" \
        -key "$WORK_DIR/keys/user_any" \
        -action "ping" >> "$CURRENT_LOG_FILE" 2>&1 &
done

# --- CORRECCIÓN AQUÍ: NO USAMOS WAIT, USAMOS SLEEP ---
# wait # <--- ESTO CAUSABA EL BLOQUEO
echo "   ...Esperando drenado de paquetes..."
sleep 3 

# ANÁLISIS DEL RATE LIMIT
# 1. Deben haber éxitos (los primeros 3)
if grep -q "Knock válido recibido" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] RateLimit: Legit traffic passed${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] RateLimit: All traffic blocked${C_NC}"
    ((TESTS_FAILED++))
fi

# 2. Deben haber rechazos
if grep -q "rate_limit_exceeded" "$CURRENT_LOG_FILE"; then
    count=$(grep -c "rate_limit_exceeded" "$CURRENT_LOG_FILE")
    echo -e "  ${C_GREEN}[PASS] RateLimit: Excess traffic blocked ($count packets dropped)${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] RateLimit: System vulnerable to DoS${C_NC}"
    ((TESTS_FAILED++))
fi

stop_daemon
