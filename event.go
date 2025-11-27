package denohttpworker

import "github.com/olebedev/emitter"

type Event string

const (
	EventSpawn  Event = "spawn"
	EventStdout Event = "stdout"
	EventStderr Event = "stderr"
)

func (w *Worker) On(event Event, listener func(data any)) func() {
	ch := w.emitter.On(string(event), func(e *emitter.Event) {
		listener(e.Args[0])
	})
	return func() {
		w.emitter.Off(string(event), ch)
	}
}
