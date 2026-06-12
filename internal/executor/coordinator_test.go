package executor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"text/template"
	"time"

	"github.com/soyunomas/ghostknock/internal/config"
)

// revertAction construye una acción con un comando de reversión que ejecuta el
// script indicado, con el delay dado.
func revertAction(t *testing.T, script string, delaySeconds int) config.Action {
	t.Helper()
	tmpl, err := template.New("revert").Parse(script)
	if err != nil {
		t.Fatalf("parse revert template: %v", err)
	}
	return config.Action{
		RevertCommand:      script,
		RevertCommandTmpl:  tmpl,
		RevertDelaySeconds: delaySeconds,
		TimeoutSeconds:     5,
	}
}

// TestCoordinatorWaitDrainsRevert verifica que Wait() bloquea hasta que la
// reversión programada (lanzada por Execute) termina realmente. Antes del fix la
// reversión escapaba del ciclo de vida y no se esperaba en el apagado.
func TestCoordinatorWaitDrainsRevert(t *testing.T) {
	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "reverted")

	c := NewCoordinator(context.Background(), 10)
	action := testAction(t, "true")
	revert := revertAction(t, "printf done > "+marker, 1)
	action.RevertCommand = revert.RevertCommand
	action.RevertCommandTmpl = revert.RevertCommandTmpl
	action.RevertDelaySeconds = revert.RevertDelaySeconds

	if err := c.Execute(action, "tester", net.ParseIP("192.0.2.1"), nil, config.Hooks{}, testDaemon()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// La reversión aún no debería haber corrido (delay de 1s).
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("revert ran before its delay elapsed")
	}

	c.Wait()

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("revert did not run before Wait returned: %v", err)
	}
}

// TestCoordinatorShutdownInterruptsRevertDelay verifica que cancelar el contexto
// (apagado) interrumpe la espera de la reversión, de modo que Wait() retorna sin
// esperar el delay completo. La reversión se ejecuta de inmediato.
func TestCoordinatorShutdownInterruptsRevertDelay(t *testing.T) {
	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "reverted")

	ctx, cancel := context.WithCancel(context.Background())
	c := NewCoordinator(ctx, 10)

	action := testAction(t, "true")
	revert := revertAction(t, "printf done > "+marker, 3600) // delay enorme
	action.RevertCommand = revert.RevertCommand
	action.RevertCommandTmpl = revert.RevertCommandTmpl
	action.RevertDelaySeconds = revert.RevertDelaySeconds

	if err := c.Execute(action, "tester", net.ParseIP("192.0.2.1"), nil, config.Hooks{}, testDaemon()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	cancel() // simula apagado

	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked on revert delay despite shutdown cancellation")
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("revert did not run after shutdown interruption: %v", err)
	}
}

// TestCoordinatorBoundsConcurrentBackgroundCommands verifica que el semáforo de
// fondo limita el número de comandos de reversión que corren simultáneamente.
func TestCoordinatorBoundsConcurrentBackgroundCommands(t *testing.T) {
	const maxConcurrent = 2
	const total = 6

	var inflight int32
	var peak int32

	c := NewCoordinator(context.Background(), maxConcurrent)

	for i := 0; i < total; i++ {
		c.goBounded(func() {
			cur := atomic.AddInt32(&inflight, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&inflight, -1)
		})
	}

	c.Wait()

	if peak > maxConcurrent {
		t.Fatalf("background concurrency peaked at %d, want <= %d", peak, maxConcurrent)
	}
	if peak == 0 {
		t.Fatal("no background tasks ran")
	}
}
