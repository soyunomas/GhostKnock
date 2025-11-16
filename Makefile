# Makefile para el proyecto GhostKnock

# ==============================================================================
# Variables de Configuración
# ==============================================================================

# Compilador de Go. ?= permite sobreescribirlo desde la línea de comandos.
GO ?= go

# Flags para la compilación. -v para modo verboso.
GOFLAGS ?= -v

# Lista de binarios que se deben construir.
TARGETS := ghostknockd ghostknock ghostknock-keygen

# Directorios de instalación estándar.
PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin
ETCDIR := /etc/ghostknock
SYSTEMDDIR := /etc/systemd/system


# ==============================================================================
# Targets Principales
# ==============================================================================

# .PHONY declara targets que no son archivos, evitando conflictos y forzando su ejecución.
.PHONY: all build clean install uninstall help

# El target por defecto, que se ejecuta al correr 'make'.
all: build

# Target para compilar todos los binarios.
build: $(TARGETS)
	@echo "✅ Todos los binarios de GhostKnock han sido compilados."

# Target para limpiar los binarios compilados del directorio actual.
clean:
	@echo "🧹 Limpiando binarios del proyecto..."
	@rm -f $(TARGETS)
	@echo "Limpieza completa."

# Target para instalar la aplicación en el sistema. Requiere permisos de superusuario.
install: build
	@echo "🚀 Instalando GhostKnock en el sistema..."
	@echo "    - Binarios en:        $(BINDIR)"
	@echo "    - Configuración en:   $(ETCDIR)"
	@echo "    - Servicio Systemd en: $(SYSTEMDDIR)"
	@install -d -m 0755 $(BINDIR)
	@install -m 0755 $(TARGETS) $(BINDIR)
	@install -d -m 0755 $(ETCDIR)
	@install -m 0644 config.yaml $(ETCDIR)/config.yaml.example
	@install -d -m 0755 $(SYSTEMDDIR)
	@install -m 0644 packaging/systemd/ghostknockd.service $(SYSTEMDDIR)/ghostknockd.service
	@echo "\n✨ ¡Instalación completada!"
	@echo "---"
	@echo "PASOS SIGUIENTES:"
	@echo "1. Edite el archivo de configuración de ejemplo:"
	@echo "   sudo nano $(ETCDIR)/config.yaml.example"
	@echo "2. Guárdelo como el archivo de configuración final:"
	@echo "   sudo cp $(ETCDIR)/config.yaml.example $(ETCDIR)/config.yaml"
	@echo "3. Recargue el demonio de Systemd para que reconozca el nuevo servicio:"
	@echo "   sudo systemctl daemon-reload"
	@echo "4. Habilite el servicio para que se inicie en el arranque:"
	@echo "   sudo systemctl enable ghostknockd.service"
	@echo "5. Inicie el servicio ahora mismo:"
	@echo "   sudo systemctl start ghostknockd.service"
	@echo "6. Verifique el estado del servicio:"
	@echo "   sudo systemctl status ghostknockd.service"


# Target para desinstalar la aplicación del sistema. Requiere permisos de superusuario.
uninstall:
	@echo "🗑️  Desinstalando GhostKnock del sistema..."
	@echo "Deteniendo y deshabilitando el servicio Systemd..."
	@systemctl stop ghostknockd.service || true
	@systemctl disable ghostknockd.service || true
	@rm -f $(SYSTEMDDIR)/ghostknockd.service
	@systemctl daemon-reload || true
	@echo "Servicio Systemd eliminado."
	@rm -f $(addprefix $(BINDIR)/, $(TARGETS))
	@echo "Binarios eliminados de $(BINDIR)."
	@if [ -d "$(ETCDIR)" ]; then \
		rm -r $(ETCDIR); \
		echo "Directorio de configuración eliminado de $(ETCDIR)."; \
	fi
	@echo "Desinstalación completa."

# Target de ayuda para mostrar los comandos disponibles.
help:
	@echo "Comandos disponibles para GhostKnock:"
	@echo "  make build       - Compila todos los binarios del proyecto."
	@echo "  make clean       - Elimina los binarios compilados."
	@echo "  make install     - (sudo) Instala binarios, config y servicio Systemd."
	@echo "  make uninstall   - (sudo) Elimina completamente la aplicación del sistema."
	@echo "  make             - Alias para 'make build'."


# ==============================================================================
# Reglas de Compilación
# ==============================================================================

# Regla de patrón genérica para construir cualquier binario listado en $(TARGETS).
# $@ es una variable automática de Make que se expande al nombre del target (ej. 'ghostknockd').
$(TARGETS):
	@echo "Building $@..."
	@$(GO) build $(GOFLAGS) -o $@ ./cmd/$@/
