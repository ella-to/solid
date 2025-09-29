```
░██████╗░█████╗░██╗░░░░░██╗██████╗░
██╔════╝██╔══██╗██║░░░░░██║██╔══██╗
╚█████╗░██║░░██║██║░░░░░██║██║░░██║
░╚═══██╗██║░░██║██║░░░░░██║██║░░██║
██████╔╝╚█████╔╝███████╗██║██████╔╝
╚═════╝░░╚════╝░╚══════╝╚═╝╚═════╝░
```

solid is a signaling/broadcast library in Golang designed to be extremly fast and low on memory (brodcast is zero allocation).

# Installation

```bash
go get ella.to/solid
```

# Usage

```golang

// create a broadcast engine
b := solid.NewBroadcast()
defer b.Close()

// create a signal
s1 := b.CreateSignal(1, false)
defer s1.Done()

// When b.Notify() is called, all signals created from this broadcast
// will be notified simultaneously. Each signal's Wait() method will
// unblock, allowing for efficient one-to-many communication patterns.
// You can create multiple signals and they will all respond to a single
// Notify() call, making this ideal for coordinating goroutines.

var wg sync.WaitGroup

wg.Add(1)

go func() {
    defer wg.Done()
    s1.Wait(context.Background())
}()

b.Notify()

wg.Wait()
```

# APIs

solid is consists of two exposed structs one is related to
