#!/bin/bash

# ==============================================================================
# GHOSTKNOCK NIGHTLY SUITE ORCHESTRATOR
# Ejecuta con: sudo ./scripts/tests_e2e/run_nightly_suite.sh
# ==============================================================================

# 1. Cargar Librería Común
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.lib.sh"

# 2. Verificar Root
if [[ $EUID -ne 0 ]]; then
   echo -e "${C_RED}Error: Necesitas sudo para pcap.${C_NC}"
   exit 1
fi

# 3. Inicialización
trap cleanup_all EXIT
setup_env

echo -e "${C_YELLOW}>>> INICIANDO SUITE DE PRUEBAS AUTOMÁTICA <<<${C_NC}"
echo "Start Time: $(date)"

# 4. Bucle de Escenarios
# Busca todos los .sh en la carpeta scenarios/ y los ejecuta
SCENARIO_DIR="$SCRIPT_DIR/scenarios"

for scenario in "$SCENARIO_DIR"/*.sh; do
    [ -e "$scenario" ] || continue
    
    # Nombre bonito para el log
    scenario_name=$(basename "$scenario" .sh)
    echo -e "\n${C_BLUE}=== EJECUTANDO FAMILIA: $scenario_name ===${C_NC}"
    echo -e "\n--- Family: $scenario_name ---" >> "$REPORT_FILE"
    
    # --- CORRECCIÓN AQUÍ: Quitamos los paréntesis ---
    source "$scenario"
    # ------------------------------------------------
    
    # Asegurar que el demonio muere entre escenarios
    stop_daemon_silent
done

# 5. Resumen Final
echo -e "\n========================================"
echo -e "RESULTADOS FINALES"
echo -e "========================================"
if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${C_GREEN}✅ TODOS LOS TESTS PASARON ($TESTS_PASSED tests)${C_NC}"
else
    echo -e "${C_RED}❌ HUBO FALLOS ($TESTS_FAILED fallos / $TESTS_PASSED éxitos)${C_NC}"
    echo "Revisa el reporte en: $REPORT_FILE"
    exit 1
fi
