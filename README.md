```
░██████╗░█████╗░██╗░░░░░██╗██████╗░
██╔════╝██╔══██╗██║░░░░░██║██╔══██╗
╚█████╗░██║░░██║██║░░░░░██║██║░░██║
░╚═══██╗██║░░██║██║░░░░░██║██║░░██║
██████╔╝╚█████╔╝███████╗██║██████╔╝
╚═════╝░░╚════╝░╚══════╝╚═╝╚═════╝░
```

<div align="center">

[![Go Reference](https://pkg.go.dev/badge/ella.to/solid.svg)](https://pkg.go.dev/ella.to/solid)
[![Go Report Card](https://goreportcard.com/badge/ella.to/solid)](https://goreportcard.com/report/ella.to/solid)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**solid** is a high-performance signaling/broadcast library for Go designed to be extremely fast and memory-efficient. The broadcast mechanism is zero-allocation and provides reliable one-to-many communication patterns for coordinating goroutines.

</div>

## Features

- **Zero-allocation broadcasting**: Efficient memory usage during broadcasts
- **Context-aware waiting**: Signals support context cancellation
- **Historical catch-up**: Signals can catch up on missed broadcasts
- **Thread-safe**: Safe for concurrent use across multiple goroutines
- **Buffered channels**: Configurable buffer sizes for different use cases

## Installation

```bash
go get ella.to/solid
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "sync"
    
    "ella.to/solid"
)

func main() {
    // Create a broadcast engine
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Create signals
    signal1 := broadcast.CreateSignal()
    signal2 := broadcast.CreateSignal()
    defer signal1.Done()
    defer signal2.Done()

    var wg sync.WaitGroup
    wg.Add(2)

    // Start goroutines waiting for signals
    go func() {
        defer wg.Done()
        signal1.Wait(context.Background())
        fmt.Println("Signal 1 received!")
    }()

    go func() {
        defer wg.Done()
        signal2.Wait(context.Background())
        fmt.Println("Signal 2 received!")
    }()

    // Notify all signals simultaneously
    broadcast.Notify()
    
    wg.Wait()
    // Output:
    // Signal 1 received!
    // Signal 2 received!
}
```

## Examples

### Basic Signal Broadcasting

```go
func basicBroadcasting() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Create multiple signals
    signals := make([]*solid.Signal, 5)
    for i := 0; i < 5; i++ {
        signals[i] = broadcast.CreateSignal()
        defer signals[i].Done()
    }

    var wg sync.WaitGroup
    wg.Add(len(signals))

    // Start workers waiting for signals
    for i, signal := range signals {
        go func(id int, s *solid.Signal) {
            defer wg.Done()
            if err := s.Wait(context.Background()); err != nil {
                fmt.Printf("Worker %d error: %v\n", id, err)
                return
            }
            fmt.Printf("Worker %d activated!\n", id)
        }(i, signal)
    }

    // Broadcast to all workers
    broadcast.Notify()
    wg.Wait()
}
```

### Context Cancellation

```go
func contextCancellation() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    signal := broadcast.CreateSignal()
    defer signal.Done()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    go func() {
        time.Sleep(3 * time.Second) // This will timeout
        broadcast.Notify()
    }()

    if err := signal.Wait(ctx); err != nil {
        if err == context.DeadlineExceeded {
            fmt.Println("Signal wait timed out")
        } else {
            fmt.Printf("Signal wait error: %v\n", err)
        }
    } else {
        fmt.Println("Signal received")
    }
}
```

### Historical Catch-up

```go
func historicalCatchup() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Send some notifications before creating signals
    broadcast.Notify() // count = 1
    broadcast.Notify() // count = 2
    broadcast.Notify() // count = 3

    // Create a signal that catches up from the beginning
    signalWithHistory := broadcast.CreateSignal(solid.WithHistory(0))
    defer signalWithHistory.Done()

    // Create a signal that only receives future notifications
    signalNoHistory := broadcast.CreateSignal()
    defer signalNoHistory.Done()

    // The signal with history will immediately have 3 pending notifications
    fmt.Println("Checking historical notifications...")
    for i := 0; i < 3; i++ {
        if err := signalWithHistory.Wait(context.Background()); err != nil {
            fmt.Printf("Error: %v\n", err)
            break
        }
        fmt.Printf("Caught up notification %d\n", i+1)
    }

    // Send a new notification
    broadcast.Notify()

    // Both signals will receive this new notification
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        signalWithHistory.Wait(context.Background())
        fmt.Println("History signal got new notification")
    }()

    go func() {
        defer wg.Done()
        signalNoHistory.Wait(context.Background())
        fmt.Println("No-history signal got notification")
    }()

    wg.Wait()
}
```

### Custom Buffer Sizes

```go
func customBufferSizes() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Create signals with different buffer sizes
    unbufferedSignal := broadcast.CreateSignal() // Default buffer size of 1
    bufferedSignal := broadcast.CreateSignal(solid.WithBufferSize(10))
    
    defer unbufferedSignal.Done()
    defer bufferedSignal.Done()

    // Send multiple rapid notifications
    for i := 0; i < 5; i++ {
        broadcast.Notify()
    }

    // Both signals will handle the notifications appropriately
    // The buffered signal can handle burst notifications better
    
    ctx := context.Background()
    
    // Drain notifications from both signals
    for {
        err := unbufferedSignal.Wait(ctx)
        if err != nil {
            break
        }
        fmt.Println("Unbuffered signal received notification")
    }
    
    for {
        err := bufferedSignal.Wait(ctx)
        if err != nil {
            break
        }
        fmt.Println("Buffered signal received notification")
    }
}
```

### Worker Pool Coordination

```go
func workerPoolCoordination() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    const numWorkers = 10
    jobs := make(chan int, 100)

    // Fill job queue
    for i := 0; i < 50; i++ {
        jobs <- i
    }
    close(jobs)

    var wg sync.WaitGroup
    wg.Add(numWorkers)

    // Create worker pool
    for i := 0; i < numWorkers; i++ {
        signal := broadcast.CreateSignal()
        
        go func(workerID int, s *solid.Signal) {
            defer wg.Done()
            defer s.Done()

            for {
                select {
                case job, ok := <-jobs:
                    if !ok {
                        return // No more jobs
                    }
                    
                    // Wait for coordination signal
                    if err := s.Wait(context.Background()); err != nil {
                        fmt.Printf("Worker %d error: %v\n", workerID, err)
                        return
                    }
                    
                    // Process job
                    fmt.Printf("Worker %d processing job %d\n", workerID, job)
                    time.Sleep(100 * time.Millisecond) // Simulate work
                }
            }
        }(i, signal)
    }

    // Coordinate workers - release them all at once
    fmt.Println("Starting all workers...")
    broadcast.Notify()

    wg.Wait()
    fmt.Println("All workers completed")
}
```

## API Reference

### Broadcast

#### `NewBroadcast(opts ...BroadCastOptFunc) *Broadcast`
Creates a new broadcast instance.

**Options:**
- `WithInitialTotal(total int64)`: Sets the initial total count for the broadcaster

#### `(*Broadcast) CreateSignal(opts ...SignalOptFunc) *Signal`
Creates a new signal subscribed to the broadcaster.

**Options:**
- `WithBufferSize(size int)`: Sets the buffer size for the signal channel (default: 1)
- `WithHistory(baseGen int64)`: Sets the base generation for historical catch-up

#### `(*Broadcast) Notify()`
Sends a signal to all subscribers, unblocking all waiting signals.

#### `(*Broadcast) Close()`
Closes the broadcaster and all associated signals.

### Signal

#### `(*Signal) Wait(ctx context.Context) error`
Blocks until a signal is received or the context is cancelled.

**Returns:**
- `nil`: Signal received successfully
- `context.Canceled`: Context was cancelled
- `context.DeadlineExceeded`: Context deadline exceeded
- `ErrSignalNotAvailable`: Signal or broadcaster is closed

#### `(*Signal) Done()`
Closes the signal and removes it from the broadcaster.

## Performance Characteristics

- **Broadcasting**: O(n) where n is the number of active signals
- **Memory**: Zero allocations during broadcast operations
- **Concurrency**: Lock-free implementation using atomic operations
- **Buffering**: Configurable channel buffer sizes to handle burst notifications

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
