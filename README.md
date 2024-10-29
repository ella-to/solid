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
s1 := b.CreateSignal(1)
defer s1.Done()

// whenever you call the b.Broadcast()
// s1.Wait() will unblock. you can have
// you can have as many signals as you want,
// and b.Broadcast() calls, all of the signals.Wait
// will be unblock

var wg sync.WaitGroup

wg.Add(1)

go func() {
    defer wg.Done()
    s1.Wait(context.Background())
}()

b.Broadcast()

wg.Wait()
```
