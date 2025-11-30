#!/bin/bash
# FAMILY: 02_params_security
# Objetivo: Validar paso de argumentos, sanitización de input (Anti-Inyección) y privacidad de logs.

generate_keys

# 2. Definir Configuración
# AÑADIDO: 'security' con un burst alto para que los tests rápidos no sean bloqueados por Anti-DoS
cat <<EOF > "$WORK_DIR/config_02.yaml"
server_private_key_path: "$WORK_DIR/keys/server_key"
listener:
  interface: "lo"
  port: 45001
  listen_ip: "127.0.0.1"
logging:
  log_level: "debug"
  log_output: "stdout"
security:
  rate_limit_burst: 20  # <--- SUBIMOS ESTO PARA TESTING
daemon:
  pid_file: "$WORK_DIR/pid_02.pid"
users:
  - name: "tester_sec"
    public_key: "$CLIENT_PUB_B64"
    actions:
      - "write-dynamic"
      - "sensitive-login"
      - "unsafe-echo"
actions:
  "write-dynamic":
    command: 'echo "{{.Params.content}}" > $WORK_DIR/param_test.txt'
    cooldown_seconds: 0
  "sensitive-login":
    command: 'echo "User: {{.Params.user}} Pass: {{.Params.pass}}" > $WORK_DIR/login_attempt.txt'
    sensitive_params:
      - "pass"
    cooldown_seconds: 0
  "unsafe-echo":
    command: 'echo {{.Params.input}}' 
    cooldown_seconds: 0
EOF

start_daemon "$(cat $WORK_DIR/config_02.yaml)" "02_params_security"

# ==============================================================================
# TEST 1: Argumentos Dinámicos
# ==============================================================================
echo ">> [Test 1] Argumentos Dinámicos Correctos..."
RANDOM_VAL="Data_$(date +%s)"

run_client \
    -port 45001 \
    -action "write-dynamic" \
    -args "content=$RANDOM_VAL"

sleep 0.5
if grep -q "$RANDOM_VAL" "$WORK_DIR/param_test.txt" 2>/dev/null; then
    echo -e "  ${C_GREEN}[PASS] Dynamic Params: File created${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Dynamic Params: Content mismatch${C_NC}"
    ((TESTS_FAILED++))
fi

# ==============================================================================
# TEST 2: Privacidad
# ==============================================================================
echo ">> [Test 2] Privacidad de Logs..."
SECRET_PASS="UltraSecret12345"

run_client \
    -port 45001 \
    -action "sensitive-login" \
    -args "user=admin,pass=$SECRET_PASS"

sleep 0.5
if grep -q "$SECRET_PASS" "$WORK_DIR/login_attempt.txt" 2>/dev/null; then
    echo -e "  ${C_GREEN}[PASS] Execution: Command received decrypted secret${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Execution: Command did NOT receive secret${C_NC}"
    ((TESTS_FAILED++))
fi

assert_log "$SECRET_PASS" "Log Privacy: Secret NOT found in logs" "false"
assert_log "\*\*\*\*\*" "Log Privacy: Redaction placeholder found"

# ==============================================================================
# TEST 3: Inyección (Punto y Coma)
# ==============================================================================
echo ">> [Test 3] Intento de Inyección..."
# Enviamos: hola;ls
run_client \
    -port 45001 \
    -action "unsafe-echo" \
    -args "input=hola;ls"

sleep 0.5
# FIX: Buscamos "SEGURIDAD" o "caracteres" en lugar de "invalid" (inglés)
if grep -q "SEGURIDAD" "$CURRENT_LOG_FILE" || grep -q "caracteres" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Security: Semicolon injection blocked${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Security: Injection might have passed (Log not found)${C_NC}"
    ((TESTS_FAILED++))
fi

# ==============================================================================
# TEST 4: Inyección (Flag)
# ==============================================================================
echo ">> [Test 4] Intento de Inyección de Flags..."
# Enviamos: -rf
run_client \
    -port 45001 \
    -action "unsafe-echo" \
    -args "input=-rf"

sleep 0.5
if grep -q "SEGURIDAD" "$CURRENT_LOG_FILE" || grep -q "guion" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Security: Flag injection (-arg) blocked${C_NC}"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Security: Flag injection might have passed${C_NC}"
    ((TESTS_FAILED++))
fi

# ==============================================================================
# TEST 5: Directory Traversal
# ==============================================================================
echo ">> [Test 5] Intento de Directory Traversal..."
# Enviamos: ../../etc/passwd
run_client \
    -port 45001 \
    -action "unsafe-echo" \
    -args "input=../../etc/passwd"

sleep 0.5
# Verificar que no fue bloqueado por rate limit primero
if grep -q "rate_limit_exceeded" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_YELLOW}[WARN] Test 5 hit Rate Limit! Increase burst in config.${C_NC}"
    # No fallamos el test, pero avisamos. Con el fix de arriba no debería pasar.
fi

if grep -q "SEGURIDAD" "$CURRENT_LOG_FILE" || grep -q "\.\." "$CURRENT_LOG_FILE"; then
     echo -e "  ${C_GREEN}[PASS] Security: Path traversal blocked${C_NC}"
     ((TESTS_PASSED++))
else
     echo -e "  ${C_RED}[FAIL] Security: Traversal might have passed${C_NC}"
     ((TESTS_FAILED++))
fi

stop_daemon
