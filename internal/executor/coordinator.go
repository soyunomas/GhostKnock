package executor

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Coordinator acota y rastrea las goroutines de fondo (reversiones programadas y
// hooks fire-and-forget) que se lanzan durante la ejecución de una acción.
//
// Sin esta coordinación esas goroutines escapaban del ciclo de vida del daemon:
//   - ignoraban el límite de concurrencia (anti-forkbomb), porque el semáforo del
//     servidor solo cubría la llamada síncrona a Execute;
//   - no se esperaban en el apagado, así que comandos como root (reversiones y
//     post-hooks) podían ejecutarse después de que el daemon reportara un cierre
//     limpio.
//
// El Coordinator resuelve ambos problemas: bounded mediante un semáforo propio y
// drenable mediante un WaitGroup. El contexto permite interrumpir las esperas de
// reversión durante el apagado para no bloquear Wait() durante todo el delay.
type Coordinator struct {
	ctx context.Context
	sem chan struct{}
	wg  sync.WaitGroup
}

// NewCoordinator crea un Coordinator acotado a maxConcurrent procesos de fondo
// simultáneos. La cancelación de ctx interrumpe las esperas de reversión
// pendientes para que el apagado pueda drenar las tareas sin esperar el delay
// completo.
func NewCoordinator(ctx context.Context, maxConcurrent int) *Coordinator {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Coordinator{
		ctx: ctx,
		sem: make(chan struct{}, maxConcurrent),
	}
}

// Wait bloquea hasta que todas las tareas de fondo rastreadas hayan terminado.
// Debe llamarse en el apagado, después de drenar las ejecuciones síncronas.
func (c *Coordinator) Wait() {
	c.wg.Wait()
}

// goTracked ejecuta fn en una goroutine rastreada por el WaitGroup y protegida
// frente a panics, para que un fallo en un hook o reversión no derribe el daemon.
func (c *Coordinator) goTracked(fn func()) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic recuperado en tarea de fondo del executor", "err", r)
			}
		}()
		fn()
	}()
}

// runBounded ejecuta fn de forma síncrona reservando primero un slot del semáforo,
// limitando el número de procesos de fondo concurrentes. No debe usarse para
// esperas largas (sleeps): adquiere el slot solo alrededor del trabajo real.
func (c *Coordinator) runBounded(fn func()) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()
	fn()
}

// goBounded lanza fn en una goroutine rastreada y acotada por el semáforo.
func (c *Coordinator) goBounded(fn func()) {
	c.goTracked(func() {
		c.runBounded(fn)
	})
}

// sleep espera d o hasta que se cancele el contexto del coordinator (apagado),
// lo que ocurra primero. Devolver antes en el apagado permite que la reversión
// se ejecute de inmediato en vez de bloquear el cierre durante todo el delay.
func (c *Coordinator) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-c.ctx.Done():
	}
}
