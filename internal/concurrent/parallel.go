package concurrent

import (
	"context"
	"sync"
)

// Result represents the result of a parallel operation
type Result[T any] struct {
	Value T
	Error error
	Index int // Original index in the input slice
}

// Task represents a function to be executed in parallel
type Task[T any] func(ctx context.Context) (T, error)

// ParallelExecute executes tasks in parallel and returns all results
// It waits for all tasks to complete, even if some fail
func ParallelExecute[T any](ctx context.Context, tasks []Task[T]) []Result[T] {
	results := make([]Result[T], len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(index int, t Task[T]) {
			defer wg.Done()
			value, err := t(ctx)
			results[index] = Result[T]{
				Value: value,
				Error: err,
				Index: index,
			}
		}(i, task)
	}

	wg.Wait()
	return results
}

// ParallelExecuteWithLimit executes tasks in parallel with a concurrency limit
// maxConcurrent specifies the maximum number of tasks running simultaneously
func ParallelExecuteWithLimit[T any](ctx context.Context, tasks []Task[T], maxConcurrent int) []Result[T] {
	if maxConcurrent <= 0 {
		maxConcurrent = len(tasks) // No limit
	}

	results := make([]Result[T], len(tasks))
	var wg sync.WaitGroup

	// Create a semaphore channel to limit concurrency
	semaphore := make(chan struct{}, maxConcurrent)

	for i, task := range tasks {
		wg.Add(1)
		go func(index int, t Task[T]) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }() // Release semaphore

			value, err := t(ctx)
			results[index] = Result[T]{
				Value: value,
				Error: err,
				Index: index,
			}
		}(i, task)
	}

	wg.Wait()
	return results
}

// ParallelMap executes a function on each item in parallel and returns the results
func ParallelMap[T any, R any](ctx context.Context, items []T, fn func(ctx context.Context, item T) (R, error)) []Result[R] {
	tasks := make([]Task[R], len(items))
	for i, item := range items {
		item := item // Capture loop variable
		tasks[i] = func(ctx context.Context) (R, error) {
			return fn(ctx, item)
		}
	}
	return ParallelExecute(ctx, tasks)
}

// ParallelMapWithLimit executes a function on each item in parallel with a concurrency limit
func ParallelMapWithLimit[T any, R any](ctx context.Context, items []T, fn func(ctx context.Context, item T) (R, error), maxConcurrent int) []Result[R] {
	tasks := make([]Task[R], len(items))
	for i, item := range items {
		item := item // Capture loop variable
		tasks[i] = func(ctx context.Context) (R, error) {
			return fn(ctx, item)
		}
	}
	return ParallelExecuteWithLimit(ctx, tasks, maxConcurrent)
}

// CollectResults separates successful results from errors
func CollectResults[T any](results []Result[T]) (values []T, errors []error) {
	values = make([]T, 0, len(results))
	errors = make([]error, 0)

	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, result.Error)
		} else {
			values = append(values, result.Value)
		}
	}

	return values, errors
}

// FirstError returns the first error from results, or nil if all succeeded
func FirstError[T any](results []Result[T]) error {
	for _, result := range results {
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// AllErrors returns all errors from results
func AllErrors[T any](results []Result[T]) []error {
	errors := make([]error, 0)
	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, result.Error)
		}
	}
	return errors
}

// HasErrors returns true if any result contains an error
func HasErrors[T any](results []Result[T]) bool {
	for _, result := range results {
		if result.Error != nil {
			return true
		}
	}
	return false
}
