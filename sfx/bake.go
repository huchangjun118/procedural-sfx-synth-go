package sfx

import "sync"

// Task is a single named bake job.
type Task struct {
	ID     string
	Params Params
}

// Result holds the outcome of one task: either Samples or Err is
// set. Results are returned in the same order as the input tasks.
type Result struct {
	ID      string
	Samples []float64
	Err     error
}

// BakeBatch renders all tasks with the given number of workers.
// Each task is rendered independently, so results are byte-identical
// for any worker count, and one failing task does not affect the
// others: its error is reported in its own Result with the task ID.
func BakeBatch(tasks []Task, workers int) []Result {
	results := make([]Result, len(tasks))
	for i, t := range tasks {
		results[i].ID = t.ID
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				samples, err := Render(&tasks[i].Params)
				if err != nil {
					results[i].Err = &BakeError{TaskID: tasks[i].ID, Err: err}
				} else {
					results[i].Samples = samples
				}
			}
		}()
	}
	for i := range tasks {
		next <- i
	}
	close(next)
	wg.Wait()
	return results
}
