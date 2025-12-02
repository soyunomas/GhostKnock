# Makefile para el proyecto GhostKnock v2.0.0

# ==============================================================================
# Variables de Configuración
# ==============================================================================

GO ?= go
GOFLAGS ?= -v

# --- Variables para el empaquetado DEB ---
VERSION := 2.0.0
ARCH := $(shell dpkg --print-architecture)
# Inyectamos la versión en tiempo de compilación para los flags -version
LDFLAGS_VERSION := -ldflags="-X main.version=$(VERSION)"

# Definición de Binarios
SERVER_BIN := ghostknockd
CLIENT_BINS := ghostknock ghostknock-keygen
# ALL_BINS agrupa todo
ALL_BINS := $(SERVER_BIN) $(CLIENT_BINS)

# Binarios para Windows (añadimos extensión .exe)
WINDOWS_BINS := $(addsuffix .exe, $(CLIENT_BINS))

# Directorios de instalación
PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin
ETCDIR := /etc/ghostknock
SYSTEMDDIR := /etc/systemd/system
LOGROTATEDIR := /etc/logrotate.d

BUILD_DIR := _build

# Nombres de paquetes
PKG_SERVER_NAME := ghostknock_$(VERSION)_$(ARCH).deb
PKG_CLIENT_NAME := ghostknock-client_$(VERSION)_$(ARCH).deb

# ==============================================================================
# Targets Públicos (Phony)
# ==============================================================================

.PHONY: all build build-linux build-windows clean \
        package-deb-server package-deb-client \
        install uninstall help

all: build-linux build-windows

# Compila todo para Linux (Host actual)
build: build-linux

# ==============================================================================
# Reglas de Compilación
# ==============================================================================

# 🐧 Linux (Nativo)
build-linux: $(ALL_BINS)
	@echo "✅ Binarios para Linux compilados."

$(ALL_BINS):
	@echo "🔨 Compilando $@ (Linux/$(ARCH)) con versión $(VERSION)..."
	@$(GO) build $(GOFLAGS) $(LDFLAGS_VERSION) -o $@ ./cmd/$@/

# 🪟 Windows (Cross-Compilation)
# Go permite compilar para Windows desde Linux simplemente configurando GOOS=windows.
build-windows: $(WINDOWS_BINS)
	@echo "✅ Binarios para Windows compilados."

# Regla de patrón para ejecutables de Windows
%.exe:
	@echo "🔨 Compilando $@ (Windows/amd64) con versión $(VERSION)..."
	@GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS_VERSION) -o $@ ./cmd/$(basename $@)/

# ==============================================================================
# Reglas de Empaquetado .DEB
# ==============================================================================

# Paquete COMPLETO (Servidor + Cliente + Keygen + Configs + Logrotate)
package-deb-server: $(ALL_BINS)
	@echo "📦 Empaquetando GHOSTKNOCK COMPLETO (Server + Tools)..."
	@rm -rf $(BUILD_DIR)/server
	@mkdir -p $(BUILD_DIR)/server/DEBIAN
	@mkdir -p $(BUILD_DIR)/server$(BINDIR)
	@mkdir -p $(BUILD_DIR)/server$(ETCDIR)
	@mkdir -p $(BUILD_DIR)/server$(SYSTEMDDIR)
	@mkdir -p $(BUILD_DIR)/server$(LOGROTATEDIR)
	
	# Metadatos
	@install -m 0644 packaging/debian/control $(BUILD_DIR)/server/DEBIAN/control
	@install -m 0755 packaging/debian/postinst $(BUILD_DIR)/server/DEBIAN/postinst
	@install -m 0755 packaging/debian/prerm $(BUILD_DIR)/server/DEBIAN/prerm
	
	# Archivos Binarios
	@install -m 0755 $(ALL_BINS) $(BUILD_DIR)/server$(BINDIR)/
	
	# Configuración de la aplicación
	# SEGURIDAD: El archivo de ejemplo se instala como 600 (lectura solo dueño)
	@install -m 0600 config.yaml $(BUILD_DIR)/server$(ETCDIR)/config.yaml.example
	
	# Configuración de Systemd
	@install -m 0644 packaging/systemd/ghostknockd.service $(BUILD_DIR)/server$(SYSTEMDDIR)/
	
	# Configuración de Logrotate
	@install -m 0644 packaging/logrotate/ghostknockd $(BUILD_DIR)/server$(LOGROTATEDIR)/ghostknockd
	
	# SEGURIDAD: El directorio de configuración debe ser inaccesible para otros.
	# Esto asegura que en el .deb el directorio tenga permisos restrictivos.
	@chmod 700 $(BUILD_DIR)/server$(ETCDIR)

	# Construcción
	@dpkg-deb --build $(BUILD_DIR)/server $(PKG_SERVER_NAME)
	@echo "✅ Paquete completo creado: $(PKG_SERVER_NAME)"

# Paquete LIGERO (Solo Cliente + Keygen)
package-deb-client: $(CLIENT_BINS)
	@echo "📦 Empaquetando CLIENTE GhostKnock (Solo herramientas)..."
	@rm -rf $(BUILD_DIR)/client
	@mkdir -p $(BUILD_DIR)/client/DEBIAN
	@mkdir -p $(BUILD_DIR)/client$(BINDIR)
	
	# Metadatos (Usamos el control-client específico)
	@install -m 0644 packaging/debian/control-client $(BUILD_DIR)/client/DEBIAN/control
	
	# Archivos
	@install -m 0755 $(CLIENT_BINS) $(BUILD_DIR)/client$(BINDIR)/
	
	# Construcción
	@dpkg-deb --build $(BUILD_DIR)/client $(PKG_CLIENT_NAME)
	@echo "✅ Paquete cliente creado: $(PKG_CLIENT_NAME)"

package-clean:
	@rm -rf $(BUILD_DIR) *.deb
	@echo "🧹 Artefactos de empaquetado eliminados."

# ==============================================================================
# Utilidades
# ==============================================================================

clean:
	@echo "🧹 Limpiando binarios..."
	@rm -f $(ALL_BINS) $(WINDOWS_BINS)
	@rm -rf $(BUILD_DIR)

install: build-linux
	@echo "🚀 Instalando GhostKnock (Completo)..."
	@install -d -m 0755 $(BINDIR) $(SYSTEMDDIR) $(LOGROTATEDIR)
	# SEGURIDAD: Creamos el directorio de configuración con modo 0700 (Solo Root)
	@install -d -m 0700 $(ETCDIR)
	
	@install -m 0755 $(ALL_BINS) $(BINDIR)
	# El archivo de ejemplo también restringido, por si acaso.
	@install -m 0600 config.yaml $(ETCDIR)/config.yaml.example
	@install -m 0644 packaging/systemd/ghostknockd.service $(SYSTEMDDIR)/ghostknockd.service
	@install -m 0644 packaging/logrotate/ghostknockd $(LOGROTATEDIR)/ghostknockd
	@echo "Instalación completa."
	@echo "🔒 NOTA DE SEGURIDAD: El directorio $(ETCDIR) ha sido blindado (chmod 700)."

uninstall:
	@systemctl stop ghostknockd.service || true
	@systemctl disable ghostknockd.service || true
	@rm -f $(SYSTEMDDIR)/ghostknockd.service
	@rm -f $(LOGROTATEDIR)/ghostknockd
	@rm -f $(addprefix $(BINDIR)/, $(ALL_BINS))
	@rm -rf $(ETCDIR)
	@echo "GhostKnock desinstalado."

help:
	@echo "GhostKnock v$(VERSION) Makefile"
	@echo ""
	@echo "Compilación:"
	@echo "  make build-linux        - Compila binarios nativos (Linux)."
	@echo "  make build-windows      - Compila binarios .exe para Windows."
	@echo "  make all                - Compila ambas plataformas."
	@echo ""
	@echo "Empaquetado (.deb):"
	@echo "  make package-deb-server - Crea .deb COMPLETO (Daemon + Client + Keygen + Logrotate)."
	@echo "  make package-deb-client - Crea .deb LIGERO (Client + Keygen)."
	@echo ""
	@echo "Gestión:"
	@echo "  make install            - Instala todo en el sistema local."
	@echo "  make clean              - Elimina binarios y temporales."
