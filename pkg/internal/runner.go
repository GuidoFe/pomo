package pomo

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type TaskRunner struct {
	count        int
	taskID       int
	taskMessage  string
	nPomodoros   int
	origDuration time.Duration
	state        State
	store        *Store
	started      time.Time
	stopped      time.Time
	pause        chan bool
	toggle       chan bool
	notifier     Notifier
	duration     time.Duration
	mu           sync.Mutex
	onEvent      []string
}

func NewMockedTaskRunner(task *Task, store *Store, notifier Notifier) (*TaskRunner, error) {
	tr := &TaskRunner{
		taskID:       task.ID,
		taskMessage:  task.Message,
		nPomodoros:   task.NPomodoros,
		origDuration: task.Duration,
		store:        store,
		state:        CREATED,
		pause:        make(chan bool),
		toggle:       make(chan bool),
		notifier:     notifier,
		duration:     task.Duration,
	}
	return tr, nil
}
func NewTaskRunner(task *Task, config *Config) (*TaskRunner, error) {
	store, err := NewStore(config.DBPath)
	if err != nil {
		return nil, err
	}
	tr := &TaskRunner{
		count:        len(task.Pomodoros),
		taskID:       task.ID,
		taskMessage:  task.Message,
		nPomodoros:   task.NPomodoros,
		origDuration: task.Duration,
		store:        store,
		state:        State(0),
		pause:        make(chan bool),
		toggle:       make(chan bool),
		notifier:     NewXnotifier(config.IconPath),
		duration:     task.Duration,
		onEvent:      config.OnEvent,
	}
	return tr, nil
}

func (t *TaskRunner) Start() {
	go t.run()
}

func (t *TaskRunner) TimeRemaining() time.Duration {
	return (t.duration - time.Since(t.started)).Truncate(time.Second)
}

func (t *TaskRunner) TimePauseDuration() time.Duration {
	return (time.Since(t.stopped)).Truncate(time.Second)
}

func (t *TaskRunner) SetState(state State) {
	t.state = state
}

// execute script command specified by `onEvent` on state change
func (t *TaskRunner) OnEvent() error {
	app, args := t.onEvent[0], t.onEvent[1:len(t.onEvent)]
	cmd := exec.Command(app, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("POMO_STATE=%s", t.state),
	)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (t *TaskRunner) run() error {
	for t.count < t.nPomodoros {
		// Create a new pomodoro where we
		// track the start / end time of
		// of this session.
		pomodoro := &Pomodoro{}
		// Start this pomodoro
		pomodoro.Start = time.Now()
		// Set state to RUNNIN
		t.SetState(RUNNING)
		// Execute onEvent command
		t.OnEvent()
		// Create a new timer
		timer := time.NewTimer(t.duration)
		// Record our started time
		t.started = pomodoro.Start
	loop:
		select {
		case <-timer.C:
			t.stopped = time.Now()
			t.count++
		case <-t.toggle:
			// Catch any toggles when we
			// are not expecting them
			goto loop
		case <-t.pause:
			timer.Stop()
			// Record the remaining time of the current pomodoro
			remaining := t.TimeRemaining()
			// Change state to PAUSED
			t.SetState(PAUSED)
			// Execute onEvent command
			t.OnEvent()
			// Wait for the user to press [p]
			<-t.pause
			// Resume the timer with previous
			// remaining time
			timer.Reset(remaining)
			// Change duration
			t.started = time.Now()
			t.duration = remaining
			// Restore state to RUNNING
			t.SetState(RUNNING)
			// Execute onEvent command
			t.OnEvent()
			goto loop
		}
		pomodoro.End = time.Now()
		err := t.store.With(func(tx *sql.Tx) error {
			return t.store.CreatePomodoro(tx, t.taskID, *pomodoro)
		})
		if err != nil {
			return err
		}
		// All pomodoros completed
		if t.count == t.nPomodoros {
			break
		}
		t.SetState(BREAKING)
		// Execute onEvent command
		t.OnEvent()
		t.notifier.Notify("Pomo", "It is time to take a break!")
		// Reset the duration incase it
		// was paused.
		t.duration = t.origDuration
		// User concludes the break
		<-t.toggle

	}
	t.notifier.Notify("Pomo", "Pomo session has completed!")
	t.SetState(COMPLETE)
	// Execute onEvent command
	t.OnEvent()
	return nil
}

func (t *TaskRunner) Toggle() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == BREAKING {
		t.toggle <- true
	}
}

func (t *TaskRunner) Pause() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == PAUSED || t.state == RUNNING {
		t.pause <- true
	}
}

func (t *TaskRunner) Status() *Status {
	return &Status{
		State:         t.state,
		Count:         t.count,
		NPomodoros:    t.nPomodoros,
		Remaining:     t.TimeRemaining(),
		Pauseduration: t.TimePauseDuration(),
	}
}
