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
	@echo "    - Binarios en: $(BINDIR)"
	@echo "    - Config en:   $(ETCDIR)"
	@install -d -m 0755 $(BINDIR)
	@install -m 0755 $(TARGETS) $(BINDIR)
	@install -d -m 0755 $(ETCDIR)
	@install -m 0644 config.yaml $(ETCDIR)/config.yaml.example
	@echo "\n✨ ¡Instalación completada!"
	@echo "---"
	@echo "PASOS SIGUIENTES:"
	@echo "1. Edite el archivo de configuración de ejemplo:"
	@echo "   sudo nano $(ETCDIR)/config.yaml.example"
	@echo "2. Guárdelo como el archivo de configuración final:"
	@echo "   sudo cp $(ETCDIR)/config.yaml.example $(ETCDIR)/config.yaml"
	@echo "3. ¡Ya puede ejecutar 'sudo ghostknockd' desde cualquier lugar!"

# Target para desinstalar la aplicación del sistema. Requiere permisos de superusuario.
uninstall:
	@echo "🗑️  Desinstalando GhostKnock del sistema..."
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
	@echo "  make install     - (sudo) Instala los binarios y la configuración en el sistema."
	@echo "  make uninstall   - (sudo) Elimina los binarios y la configuración del sistema."
	@echo "  make             - Alias para 'make build'."


# ==============================================================================
# Reglas de Compilación
# ==============================================================================

# Regla de patrón genérica para construir cualquier binario listado en $(TARGETS).
# $@ es una variable automática de Make que se expande al nombre del target (ej. 'ghostknockd').
$(TARGETS):
	@echo "Building $@..."
	@$(GO) build $(GOFLAGS) -o $@ ./cmd/$@/
