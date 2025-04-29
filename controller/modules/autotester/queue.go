package autotester

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

// Task represents a single test or calibration request.
type Task struct {
	ID    string `json:"id"`
	Param string `json:"param"`
	Code  byte   `json:"code"`
	Time  int64  `json:"ts"`
}

// storeIface is the minimal subset of the controller store we need.
type storeIface interface {
	List(bucket string, fn func(string, []byte) error) error
	Create(bucket string, fn func(string) interface{}) error
	Delete(bucket, id string) error
}

// Queue manages a persistent FIFO queue of tasks.
type Queue struct {
	store   storeIface
	mu      sync.Mutex
	cond    *sync.Cond
	current *Task
}

// NewQueue creates a new Queue backed by the given store.
// Pass in your controller’s Store() return value here.
func NewQueue(store storeIface) (*Queue, error) {
	q := &Queue{store: store}
	q.cond = sync.NewCond(&q.mu)
	return q, nil
}

// AddTask enqueues a new task if no duplicate is queued or running.
func (q *Queue) AddTask(param string, code byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Reject if already in progress
	if q.current != nil && q.current.Param == param {
		return errors.New("task for " + param + " already in progress")
	}

	// Reject if already queued
	if err := q.store.List(queueBucket, func(_ string, v []byte) error {
		var t Task
		if err := json.Unmarshal(v, &t); err == nil && t.Param == param {
			return errors.New("duplicate")
		}
		return nil
	}); err != nil {
		return errors.New("task for " + param + " already queued")
	}

	// Persist new task
	task := Task{Param: param, Code: code, Time: time.Now().Unix()}
	fn := func(id string) interface{} {
		task.ID = id
		return &task
	}
	if err := q.store.Create(queueBucket, fn); err != nil {
		return err
	}

	// Wake up the worker
	q.cond.Signal()
	return nil
}

// RemoveTask cancels a queued task for the given param.
func (q *Queue) RemoveTask(param string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Find the queued task
	var deleteID string
	_ = q.store.List(queueBucket, func(id string, v []byte) error {
		var t Task
		if err := json.Unmarshal(v, &t); err == nil && t.Param == param {
			deleteID = id
			return errors.New("found")
		}
		return nil
	})
	if deleteID == "" {
		return errors.New("no queued task for " + param)
	}

	// Don't cancel if currently running
	if q.current != nil && q.current.Param == param {
		return errors.New("cannot cancel, task for " + param + " is running")
	}

	return q.store.Delete(queueBucket, deleteID)
}

// ListTasks returns all pending tasks in FIFO order.
func (q *Queue) ListTasks() ([]Task, error) {
	tasks := []Task{}
	if err := q.store.List(queueBucket, func(_ string, v []byte) error {
		var t Task
		if err := json.Unmarshal(v, &t); err == nil {
			tasks = append(tasks, t)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Sort by enqueue timestamp, oldest first
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Time < tasks[j].Time
	})
	return tasks, nil
}

// ProcessTasks runs the given worker for each task, in order.
// It blocks & waits for new tasks by using a condition variable.
func (q *Queue) ProcessTasks(worker func(Task)) {
	for {
		q.mu.Lock()
		// Find the next (oldest) task
		var next *Task
		var nextKey string
		_ = q.store.List(queueBucket, func(id string, v []byte) error {
			var t Task
			if err := json.Unmarshal(v, &t); err == nil {
				if next == nil || t.Time < next.Time {
					next = &t
					nextKey = id
				}
			}
			return nil
		})

		// If none, wait for AddTask to signal
		if next == nil {
			q.cond.Wait()
			q.mu.Unlock()
			continue
		}

		// Dequeue it
		_ = q.store.Delete(queueBucket, nextKey)
		q.current = next
		q.mu.Unlock()

		// Execute the task
		worker(*next)

		// Mark done
		q.mu.Lock()
		q.current = nil
		q.mu.Unlock()
	}
}
