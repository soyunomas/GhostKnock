#!/bin/bash

# ==============================================================================
# GHOSTKNOCK COMMON LIBRARY
# Funciones compartidas para la suite de pruebas E2E.
# ==============================================================================

# Colores
C_GREEN='\033[0;32m'
C_RED='\033[0;31m'
C_YELLOW='\033[1;33m'
C_BLUE='\033[0;34m'
C_NC='\033[0m'

# Rutas Relativas (Asumiendo ejecución desde root del proyecto o scripts/)
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
BIN_SERVER="$PROJECT_ROOT/ghostknockd"
BIN_CLIENT="$PROJECT_ROOT/ghostknock"
BIN_KEYGEN="$PROJECT_ROOT/ghostknock-keygen"

# Directorio de Trabajo Efímero para ESTA ejecución
WORK_DIR="/tmp/gk_test_$(date +%s)"
REPORT_FILE="$WORK_DIR/final_report.txt"

# Variables Globales de Estado
CURRENT_DAEMON_PID=""
CURRENT_LOG_FILE=""
TESTS_PASSED=0
TESTS_FAILED=0

# ------------------------------------------------------------------------------
# SETUP Y TEARDOWN
# ------------------------------------------------------------------------------

setup_env() {
    echo -e "${C_BLUE}[INIT] Preparando entorno en: $WORK_DIR${C_NC}"
    mkdir -p "$WORK_DIR/keys"
    
    # 1. Compilación (Solo una vez)
    if [ ! -f "$BIN_SERVER" ] || [ ! -f "$BIN_CLIENT" ]; then
        echo -e "${C_YELLOW}[BUILD] Compilando binarios frescos...${C_NC}"
        cd "$PROJECT_ROOT" && make build-linux > /dev/null 2>&1
        if [ $? -ne 0 ]; then
            echo -e "${C_RED}[FATAL] Error compilando. Abortando.${C_NC}"
            exit 1
        fi
        cd - > /dev/null
    fi
    
    # Inicializar reporte
    echo "GHOSTKNOCK AUTOMATED TEST REPORT - $(date)" > "$REPORT_FILE"
    echo "=============================================" >> "$REPORT_FILE"
}

cleanup_all() {
    stop_daemon_silent
    echo -e "\n${C_BLUE}[CLEANUP] Borrando archivos temporales...${C_NC}"
    # rm -rf "$WORK_DIR" # Descomentar para borrar trazas, dejar comentado para debug
    echo -e "Reporte guardado en: $REPORT_FILE"
}

# ------------------------------------------------------------------------------
# GESTIÓN DE CLAVES
# ------------------------------------------------------------------------------

generate_keys() {
    # Genera 3 pares de claves: Server, Client, Attacker
    local key_dir="$WORK_DIR/keys"
    mkdir -p "$key_dir"
    
    # Server
    "$BIN_KEYGEN" -o "$key_dir/server_key" > /dev/null 2>&1
    SERVER_PUB_B64=$(base64 -w 0 < "$key_dir/server_key.pub")
    
    # Client (Legit)
    "$BIN_KEYGEN" -o "$key_dir/client_key" > /dev/null 2>&1
    CLIENT_PUB_B64=$(base64 -w 0 < "$key_dir/client_key.pub")
    
    # Attacker (Bad)
    "$BIN_KEYGEN" -o "$key_dir/attacker_key" > /dev/null 2>&1
    # No guardamos la del atacante en variable, solo el archivo
}

# ------------------------------------------------------------------------------
# GESTIÓN DEL DEMONIO
# ------------------------------------------------------------------------------

start_daemon() {
    local config_content="$1"
    local scenario_name="$2"
    
    CURRENT_LOG_FILE="$WORK_DIR/${scenario_name}.log"
    local config_file="$WORK_DIR/${scenario_name}.yaml"
    
    # Escribir config
    echo "$config_content" > "$config_file"
    
    # Arrancar (Redirigiendo stdout/stderr al archivo log del escenario)
    # Nota: Usamos log_output en config, pero por seguridad capturamos stderr del proceso también
    "$BIN_SERVER" -config "$config_file" > "$CURRENT_LOG_FILE" 2>&1 &
    CURRENT_DAEMON_PID=$!
    
    # Esperar warmup del pcap
    sleep 1
    
    # Verificar vida
    if ! kill -0 $CURRENT_DAEMON_PID 2>/dev/null; then
        echo -e "${C_RED}[FATAL] El demonio murió al nacer. Log:${C_NC}"
        cat "$CURRENT_LOG_FILE"
        exit 1
    fi
}

stop_daemon() {
    if [ -n "$CURRENT_DAEMON_PID" ]; then
        kill "$CURRENT_DAEMON_PID" 2>/dev/null
        wait "$CURRENT_DAEMON_PID" 2>/dev/null
        CURRENT_DAEMON_PID=""
    fi
}

stop_daemon_silent() {
    stop_daemon > /dev/null 2>&1
}

# ------------------------------------------------------------------------------
# HELPERS DE TEST
# ------------------------------------------------------------------------------

run_client() {
    # Wrapper para lanzar el cliente
    # Argumentos: -action X -args Y -key Z (opcional)
    local key_file="$WORK_DIR/keys/client_key" # Default
    
    # Si se pasan argumentos, se usan tal cual
    "$BIN_CLIENT" \
        -host "127.0.0.1" \
        -port 45000 \
        -server-pubkey "$WORK_DIR/keys/server_key.pub" \
        -key "$key_file" \
        "$@" >> "$CURRENT_LOG_FILE" 2>&1 
        # Redirigimos salida del cliente al mismo log para tener traza completa
}

assert_log() {
    local search_term="$1"
    local test_name="$2"
    local should_exist="${3:-true}" # "true" = debe existir, "false" = NO debe existir
    
    local found=false
    if grep -q "$search_term" "$CURRENT_LOG_FILE"; then
        found=true
    fi
    
    if [ "$should_exist" == "true" ] && [ "$found" == "true" ]; then
        echo -e "  ${C_GREEN}[PASS] $test_name${C_NC}"
        echo "[PASS] $test_name" >> "$REPORT_FILE"
        ((TESTS_PASSED++))
    elif [ "$should_exist" == "false" ] && [ "$found" == "false" ]; then
        echo -e "  ${C_GREEN}[PASS] $test_name (Correctly absent)${C_NC}"
        echo "[PASS] $test_name" >> "$REPORT_FILE"
        ((TESTS_PASSED++))
    else
        echo -e "  ${C_RED}[FAIL] $test_name${C_NC}"
        echo "         Esperado: '$search_term'. Encontrado: $found"
        echo "[FAIL] $test_name" >> "$REPORT_FILE"
        ((TESTS_FAILED++))
    fi
}
