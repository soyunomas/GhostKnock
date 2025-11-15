# 👻 GhostKnock

**GhostKnock** es una reimplementación moderna del concepto de *port knocking*, diseñada para ser **segura**, **flexible** y **discreta**.
A diferencia de técnicas tradicionales basadas en secuencias de paquetes TCP/UDP —fáciles de detectar o falsificar— GhostKnock utiliza **criptografía de clave pública (`ed25519`)** para autenticar y autorizar la ejecución de acciones remotas mediante **un único paquete UDP**.

El sistema está escrito en **Go**, generando binarios autocontenidos y sin dependencias externas, perfectos para despliegues en sistemas Linux.

---

## ✨ Características Principales

* 🔐 **Seguridad Criptográfica:**
  Cada knock es un payload firmado digitalmente. El servidor verifica tanto su autenticidad como su integridad con la clave pública del usuario.

* 🛡 **Defensa Anti-Replay:**
  Protección en dos capas:

  1. ⏱ Ventana de tiempo estricta.
  2. 🔁 Cooldown por acción para evitar ejecuciones repetidas.

* 🧩 **Configuración Declarativa:**
  Un solo archivo `config.yaml` define usuarios, claves públicas y acciones permitidas.

* 🕵️ **Bajo Perfil:**
  Escucha pasiva mediante `pcap` sin abrir puertos → **invisible para escaneos de red**.

* ⚙️ **Acciones Flexibles:**
  Ejecuta comandos del sistema definidos en configuración, con plantillas seguras (`text/template`) y capacidades de rollback automático.

---

## 🧠 Cómo Funciona

GhostKnock se compone de tres herramientas:

1. **`ghostknock-keygen`** → Generación de claves `ed25519`.
2. **`ghostknockd`** → Demonio del servidor: valida knocks y ejecuta acciones permitidas.
3. **`ghostknock`** → Cliente CLI: firma y envía el paquete UDP.

---

## 🛠 Instalación y Uso

### 1. Prerrequisitos

* Go 1.18+
* libpcap

  * Debian/Ubuntu → `sudo apt install libpcap-dev`
  * RHEL/CentOS → `sudo yum install libpcap-devel`

---

### 2. Compilación

```bash
go build -o ghostknockd ./cmd/ghostknockd/
go build -o ghostknock ./cmd/ghostknock/
go build -o ghostknock-keygen ./cmd/ghostknock-keygen/
```

---

### 3. Configuración

#### 🔑 Generar claves

```bash
./ghostknock-keygen -o mi_portatil_key
mv mi_portatil_key id_ed25519
```

#### 🧾 Crear `config.yaml`

```yaml
listener:
  interface: "lo"
  port: 3001

users:
  - name: "mi_portatil"
    public_key: "PEGA_AQUI_TU_CLAVE_PUBLICA_BASE64"
    actions:
      - "create-test-file"

actions:
  "create-test-file":
    command: "echo \"Knock válido de {{.SourceIP}} recibido a las $(date)\" > /tmp/ghostknock_success.txt"
    revert_command: "rm /tmp/ghostknock_success.txt"
    revert_delay_seconds: 15
```

---

### 4. Ejecución

**Terminal 1 — Servidor:**

```bash
sudo ./ghostknockd
```

**Terminal 2 — Cliente:**

```bash
./ghostknock -host 127.0.0.1 -action create-test-file
```

---

### 5. Verificación

```bash
cat /tmp/ghostknock_success.txt
```

Después de 15 segundos:

```bash
ls /tmp/ghostknock_success.txt
# Debería no existir
```

---

## 🗺 Hoja de Ruta del Proyecto

### 🟢 Fase I: Estado Inicial — **COMPLETADA**

* [x] Configuración en `config.yaml`
* [x] Soporte para múltiples claves públicas
* [x] Validación de firmas

### 🟢 Fase II: Interacción Segura — **COMPLETADA**

* [x] Captura de IP de origen
* [x] Paquete `executor`
* [x] Plantillas seguras + acciones de reversión
* [x] Integración completa en servidor

### 🟡 Fase III: Defensa Activa — **EN PROGRESO**

* [x] Sistema anti-replay avanzado
* [ ] Rate limiting por IP
* [ ] Logging estructurado (JSON)
* [ ] Graceful shutdown

### 🔵 Fase IV: Usabilidad Avanzada — **PENDIENTE**

* [ ] Makefile
* [ ] Configuración cliente mejorada

---

## 📄 Licencia

Este proyecto está bajo la **Licencia MIT**.
Consulta el archivo `LICENSE` para más información.
