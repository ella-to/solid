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

**solid** is a high-performance signaling/broadcast library for Go designed to be extremely fast and memory-efficient. The broadcast mechanism provides reliable one-to-many communication patterns for coordinating goroutines.

</div>

## Features

- **Two implementations**: Choose between channel-based or sync.Cond-based based on your needs
- **Interface-based design**: Easy to swap implementations without changing code
- **Context-aware waiting**: Signals support context cancellation
- **Historical catch-up**: Signals can catch up on missed broadcasts
- **Thread-safe**: Safe for concurrent use across multiple goroutines
- **Zero-allocation fast path**: Efficient memory usage during broadcasts

## Installation

```bash
go get ella.to/solid
```

## Choosing an Implementation

solid provides two implementations of the `Broadcast` interface:

| Feature | Channel-based (`NewBroadcast`) | sync.Cond-based (`NewBroadcastCond`) |
|---------|-------------------------------|--------------------------------------|
| **Single Signal Throughput** | ~550 ns/op | **~42 ns/op** (13x faster) |
| **Broadcast to 100 Signals** | ~1,470 ns/op | **~900 ns/op** (40% faster) |
| **Allocations per Notify** | 0 | 1 (896 bytes) |
| **Context Cancellation** | Native (efficient) | Spawns goroutine per Wait |
| **Best For** | General use, frequent cancellation | High throughput, rare cancellation |

### When to use `NewBroadcast` (Channel-based)

- ✅ General-purpose signaling
- ✅ Frequent context cancellations/timeouts
- ✅ Zero allocations is critical
- ✅ Simpler mental model (Go channels)

### When to use `NewBroadcastCond` (sync.Cond-based)

- ✅ Maximum throughput required
- ✅ High-frequency notify/wait cycles
- ✅ Context cancellation is rare
- ✅ Broadcasting to many signals simultaneously

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
    // Create a broadcast engine (use NewBroadcastCond for higher performance)
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
}
```

## Switching Implementations

Since both implementations satisfy the same interfaces, you can easily switch:

```go
// Channel-based (general purpose)
var broadcast solid.Broadcast = solid.NewBroadcast()

// sync.Cond-based (high performance)
var broadcast solid.Broadcast = solid.NewBroadcastCond()

// The rest of your code works identically
signal := broadcast.CreateSignal()
defer signal.Done()

broadcast.Notify()
signal.Wait(context.Background())
```

## API Reference

### Interfaces

#### `Signal`

```go
type Signal interface {
    // Wait blocks until a signal is received or the context is done.
    Wait(ctx context.Context) error
    
    // Done closes the signal and removes it from the broadcaster.
    Done()
}
```

#### `Broadcast`

```go
type Broadcast interface {
    // CreateSignal creates a new signal subscribed to the broadcaster.
    CreateSignal(opts ...SignalOption) Signal
    
    // Notify sends a signal to all subscribers.
    Notify()
    
    // Close closes the broadcaster and all signals.
    Close()
}
```

### Constructors

#### `NewBroadcast(opts ...BroadcastOption) Broadcast`
Creates a new channel-based broadcaster.

#### `NewBroadcastCond(opts ...BroadcastOption) Broadcast`
Creates a new sync.Cond-based broadcaster (higher performance).

### Options

#### Signal Options

```go
// WithBufferSize sets the channel buffer size (channel-based only)
// Default is 1
signal := broadcast.CreateSignal(solid.WithBufferSize(10))

// WithHistory sets the base generation for historical catch-up
// - 0 (default): catches up on all historical broadcasts
// - -1: skips all history (starts fresh)  
// - N > 0: catches up on broadcasts since generation N
signal := broadcast.CreateSignal(solid.WithHistory(-1)) // No history
signal := broadcast.CreateSignal(solid.WithHistory(0))  // All history
signal := broadcast.CreateSignal(solid.WithHistory(50)) // Since generation 50
```

#### Broadcast Options

```go
// WithInitialTotal sets the initial total count
// Useful for testing or restoring state
broadcast := solid.NewBroadcast(solid.WithInitialTotal(100))
```

### Return Values

`Signal.Wait(ctx)` returns:
- `nil`: Signal received successfully
- `context.Canceled`: Context was cancelled
- `context.DeadlineExceeded`: Context deadline exceeded
- `solid.ErrSignalNotAvailable`: Signal or broadcaster is closed

## Examples

### Context Cancellation

```go
func withTimeout() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    signal := broadcast.CreateSignal()
    defer signal.Done()

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    err := signal.Wait(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("Timed out waiting for signal")
    }
}
```

### Historical Catch-up

```go
func catchUpOnHistory() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Send notifications before creating signal
    for i := 0; i < 100; i++ {
        broadcast.Notify()
    }

    // This signal will have 100 pending notifications
    signal := broadcast.CreateSignal(solid.WithHistory(0))
    defer signal.Done()

    // Consume all historical notifications
    for i := 0; i < 100; i++ {
        signal.Wait(context.Background())
    }
}
```

### Skip History

```go
func skipHistory() {
    broadcast := solid.NewBroadcast()
    defer broadcast.Close()

    // Send notifications before creating signal
    for i := 0; i < 100; i++ {
        broadcast.Notify()
    }

    // This signal ignores all historical notifications
    signal := broadcast.CreateSignal(solid.WithHistory(-1))
    defer signal.Done()

    // Will block until next Notify() call
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    err := signal.Wait(ctx) // Will timeout
    fmt.Println("Timeout:", err == context.DeadlineExceeded)
}
```

### Worker Pool Coordination

```go
func workerPool() {
    broadcast := solid.NewBroadcastCond() // High performance for many signals
    defer broadcast.Close()

    const numWorkers = 100
    var wg sync.WaitGroup
    wg.Add(numWorkers)

    for i := 0; i < numWorkers; i++ {
        signal := broadcast.CreateSignal()
        go func(id int, s solid.Signal) {
            defer wg.Done()
            defer s.Done()

            s.Wait(context.Background())
            fmt.Printf("Worker %d started\n", id)
        }(i, signal)
    }

    time.Sleep(10 * time.Millisecond) // Let workers start

    // Start all workers simultaneously
    broadcast.Notify()
    wg.Wait()
}
```

## Benchmarks

Run benchmarks with:

```bash
go test -bench=. -benchmem
```

Example output (Apple M2 Pro):

```
BenchmarkCond1Signal-12                 27988600    42.45 ns/op     0 B/op    0 allocs/op
BenchmarkCondBroadcast100Signals-12      1289343   904.7 ns/op    896 B/op    1 allocs/op
Benchmark1Singal-12                      2166340   554.4 ns/op      0 B/op    0 allocs/op
BenchmarkBroadcast100Signals-12           825218  1518 ns/op        0 B/op    0 allocs/op
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
