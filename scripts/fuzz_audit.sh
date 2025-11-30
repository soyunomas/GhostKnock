#!/bin/bash

# Archivos temporales para capturar la salida del fuzzer
LOG_PROTO="/tmp/gk_fuzz_proto.log"
LOG_LISTENER="/tmp/gk_fuzz_listener.log"
rm -f $LOG_PROTO $LOG_LISTENER

# Colores
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Variables
PROTO_STATUS="PENDING"
LISTENER_STATUS="PENDING"
EXIT_CODE=0
TOTAL_EXECS=0

echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}   🛡️  GHOSTKNOCK AUTOMATED SECURITY AUDIT (Fuzzing)  ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo -e "Fecha de auditoría: $(date)"
echo ""

# Función auxiliar para extraer el número de ejecuciones del log
extract_execs() {
    # Busca la última línea que contenga "execs:", extrae el número después.
    # Formato esperado: "fuzz: elapsed: 30s, execs: 123456 ..."
    if [ -f "$1" ]; then
        grep "execs:" "$1" | tail -n 1 | sed -E 's/.*execs: ([0-9]+).*/\1/'
    else
        echo "0"
    fi
}

# ------------------------------------------------------------------
# 1. TEST DE PROTOCOLO
# ------------------------------------------------------------------
echo -e "${YELLOW}[1/2] Fuzzing: Protocolo (Deserialización JSON)...${NC}"
echo "      Buscando pánicos ante payloads corruptos..."

# Ejecutamos, redirigimos stderr a stdout, mostramos en pantalla Y guardamos en archivo
set -o pipefail
if go test ./internal/protocol -fuzz=FuzzDeserializePayload -fuzztime=30s 2>&1 | tee $LOG_PROTO; then
    PROTO_STATUS="${GREEN}PASSED ✅${NC}"
else
    PROTO_STATUS="${RED}FAILED ❌${NC}"
    EXIT_CODE=1
fi

# Calcular ejecuciones
COUNT=$(extract_execs $LOG_PROTO)
TOTAL_EXECS=$((TOTAL_EXECS + COUNT))
echo ""

# ------------------------------------------------------------------
# 2. TEST DE LISTENER
# ------------------------------------------------------------------
echo -e "${YELLOW}[2/2] Fuzzing: Listener (Validación de Paquetes)...${NC}"
echo "      Verificando resistencia a paquetes malformados y límites de 1KB..."

if go test ./internal/listener -fuzz=FuzzExtractPacketInfo -fuzztime=30s 2>&1 | tee $LOG_LISTENER; then
    LISTENER_STATUS="${GREEN}PASSED ✅${NC}"
else
    LISTENER_STATUS="${RED}FAILED ❌${NC}"
    EXIT_CODE=1
fi

# Calcular ejecuciones
COUNT=$(extract_execs $LOG_LISTENER)
TOTAL_EXECS=$((TOTAL_EXECS + COUNT))
echo ""

# Limpieza
rm -f $LOG_PROTO $LOG_LISTENER

# Cálculo para formato "X Millones"
# Usamos awk para permitir decimales (ej. 4.8 Millones)
MILLIONS=$(awk "BEGIN {printf \"%.2f\", $TOTAL_EXECS/1000000}")

# ------------------------------------------------------------------
# RESUMEN Y CERTIFICACIÓN
# ------------------------------------------------------------------
echo -e "${BLUE}=====================================================${NC}"
echo -e "${BLUE}               RESUMEN DE RESULTADOS                 ${NC}"
echo -e "${BLUE}=====================================================${NC}"
echo ""
echo -e "  COMPONENT           | TEST TYPE      | STATUS"
echo -e "  ------------------- | -------------- | -----------"
echo -e "  internal/protocol   | Fuzzing (JSON) | $PROTO_STATUS"
echo -e "  internal/listener   | Fuzzing (Net)  | $LISTENER_STATUS"
echo ""

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}   🎉 AUDITORÍA COMPLETADA: EL SISTEMA ES ROBUSTO  🎉${NC}"
    echo ""
    echo -e "${YELLOW}   CERTIFICACIÓN DE ROBUSTEZ:${NC}"
    echo "   Se ha integrado una suite de pruebas de Fuzzing (Go 1.18+) para el"
    echo "   listener de red y el deserializador del protocolo. La estabilidad del"
    echo "   sistema ante entradas maliciosas o corruptas ha sido validada"
    echo -e "   empíricamente con más de ${GREEN}${MILLIONS} millones${NC} de casos de prueba"
    echo "   sin fallos (panics)."
else
    echo -e "${RED}   ⚠️  AUDITORÍA FALLIDA: SE DETECTARON ERRORES    ⚠️${NC}"
fi
echo -e "${BLUE}=====================================================${NC}"

exit $EXIT_CODE
