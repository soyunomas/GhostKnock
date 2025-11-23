#!/bin/bash
# FAMILY: 04_logic_timing
# Objetivo: Verificar Cooldowns y Timeouts.

generate_keys

cat <<EOF > "$WORK_DIR/config_04.yaml"
server_private_key_path: "$WORK_DIR/keys/server_key"
listener:
  interface: "lo"
  port: 45003
  listen_ip: "127.0.0.1"
logging:
  log_level: "debug"
  log_output: "stdout"
daemon:
  pid_file: "$WORK_DIR/pid_04.pid"
users:
  - name: "tester_timing"
    public_key: "$CLIENT_PUB_B64"
    actions:
      - "fast-action"
      - "slow-action"
actions:
  "fast-action":
    command: 'echo FAST_OK'
    cooldown_seconds: 3
  "slow-action":
    command: 'sleep 5 && echo SLOW_OK'
    timeout_seconds: 2
    cooldown_seconds: 0
EOF

start_daemon "$(cat $WORK_DIR/config_04.yaml)" "04_logic_timing"

# TEST 1: Cooldown OK
echo ">> [Test 1] Cooldown: Ejecución Inicial..."
run_client -port 45003 -action "fast-action"
sleep 0.5
if grep -q "Knock válido.*fast-action" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Cooldown: First call accepted${C_NC}"
    echo "[PASS] Cooldown: First call accepted" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Cooldown: First call failed${C_NC}"
    echo "[FAIL] Cooldown: First call failed" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
fi

# TEST 2: Cooldown Fail
echo ">> [Test 2] Cooldown: Ejecución Inmediata..."
run_client -port 45003 -action "fast-action"
sleep 0.5
if grep -q "cooldown_active" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Cooldown: Rapid execution blocked${C_NC}"
    echo "[PASS] Cooldown: Rapid execution blocked" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    count=$(grep -c "Knock válido.*fast-action" "$CURRENT_LOG_FILE")
    if [ "$count" -eq 1 ]; then
        echo -e "  ${C_GREEN}[PASS] Cooldown: Blocked (Log count correct)${C_NC}"
        echo "[PASS] Cooldown: Blocked (Log count correct)" >> "$REPORT_FILE"
        ((TESTS_PASSED++))
    else
        echo -e "  ${C_RED}[FAIL] Cooldown: Action executed twice! ($count times)${C_NC}"
        echo "[FAIL] Cooldown: Action executed twice" >> "$REPORT_FILE"
        ((TESTS_FAILED++))
    fi
fi

# TEST 3: Cooldown Expired
echo ">> [Test 3] Cooldown: Esperando expiración (3.5s)..."
sleep 3.5
run_client -port 45003 -action "fast-action"
sleep 0.5
count=$(grep -c "Knock válido.*fast-action" "$CURRENT_LOG_FILE")
if [ "$count" -eq 2 ]; then
    echo -e "  ${C_GREEN}[PASS] Cooldown: Action accepted after wait${C_NC}"
    echo "[PASS] Cooldown: Action accepted after wait" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Cooldown: Action failed after wait (Count: $count)${C_NC}"
    echo "[FAIL] Cooldown: Action failed after wait" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
fi

# TEST 4: Timeout
echo ">> [Test 4] Timeout: Ejecutando comando lento..."
run_client -port 45003 -action "slow-action"
echo "   ...Esperando timeout (3s)..."
sleep 3

if grep -q "timeout_seconds" "$CURRENT_LOG_FILE" || grep -q "killed" "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_GREEN}[PASS] Timeout: Process killed by daemon${C_NC}"
    echo "[PASS] Timeout: Process killed by daemon" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
else
    echo -e "  ${C_RED}[FAIL] Timeout: No timeout log found${C_NC}"
    echo "[FAIL] Timeout: No timeout log found" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
fi

if grep -q 'output="SLOW_OK"' "$CURRENT_LOG_FILE"; then
    echo -e "  ${C_RED}[FAIL] Timeout: Command completed (FAIL)${C_NC}"
    echo "[FAIL] Timeout: Command completed (FAIL)" >> "$REPORT_FILE"
    ((TESTS_FAILED++))
else
    echo -e "  ${C_GREEN}[PASS] Timeout: Command did NOT finish (Success)${C_NC}"
    echo "[PASS] Timeout: Command did NOT finish" >> "$REPORT_FILE"
    ((TESTS_PASSED++))
fi

stop_daemon
